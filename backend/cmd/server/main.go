// PortalT 后端服务入口 (Phase 5)
//
// 负责依赖装配：数据库（OpenDBFromEnv）→ 仓储 → 认证/JWT →
// 管理员引导 → Gin 路由，最后启动 HTTP 服务。
package main

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"portalt/internal/adapters"
	authadapter "portalt/internal/adapters/auth"
	"portalt/internal/adapters/gormstore"
	"portalt/internal/api"
	"portalt/internal/api/middleware"
	"portalt/internal/api/v1"
	"portalt/internal/domain/services"
	"portalt/internal/plugins"
	"portalt/internal/plugins/examples/cron"
	"portalt/internal/plugins/examples/esxiadmin"
)

// builtinPlugins 内置原生插件列表（示例插件见 internal/plugins/examples）。
func builtinPlugins() []plugins.Plugin {
	return []plugins.Plugin{
		esxiadmin.New(),
		cron.New(),
	}
}

const (
	// AppVersion 当前版本号
	AppVersion = "v0.1"
	// listenAddr HTTP 监听地址
	listenAddr = ":8080"
)

func main() {
	log.Println("PortalT", AppVersion, "starting...")

	ctx := context.Background()

	// 数据库与仓储
	db, err := adapters.OpenDBFromEnv(ctx)
	if err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	userRepo := gormstore.NewUserRepository(db)
	vmRepo := gormstore.NewVMRepository(db)
	pluginRepo := gormstore.NewPluginRepository(db)

	// 认证与令牌
	secret := envOr("JWT_SECRET", "")
	tm := authadapter.NewJWTManager(secret,
		envSeconds("JWT_ACCESS_TTL", 900),
		envSeconds("JWT_REFRESH_TTL", 7*24*3600),
	)
	authProvider := authadapter.NewLocalProvider(userRepo)

	// 管理员初始账号引导
	if err := authadapter.EnsureAdminUser(ctx, userRepo,
		envOr("ADMIN_USERNAME", "admin"),
		envOr("ADMIN_PASSWORD", "admin123"),
	); err != nil {
		log.Fatalf("管理员账号引导失败: %v", err)
	}
	log.Printf("管理员账号已就绪（%s）", envOr("ADMIN_USERNAME", "admin"))

	// 角色权限矩阵引导（内置三角色种子 + 权限加载器）
	roleRepo := gormstore.NewRoleRepository(db)
	if err := services.EnsureDefaultRoles(ctx, roleRepo); err != nil {
		log.Fatalf("角色权限引导失败: %v", err)
	}
	roleLoader := middleware.NewRoleLoader(roleRepo)
	log.Printf("角色权限矩阵已就绪")

	// 原生插件注册表（启动时注册完毕；同步到 plugins 表供菜单与权限管理）
	native := plugins.NewRegistry()
	for _, p := range builtinPlugins() {
		if err := native.Register(p); err != nil {
			log.Fatalf("原生插件注册失败: %v", err)
		}
	}
	if n, err := services.SyncNativePlugins(ctx, pluginRepo, native); err != nil {
		log.Printf("警告: 原生插件同步失败: %v", err)
	} else if n > 0 {
		log.Printf("原生插件已同步: %d 个", n)
	}

	// 虚拟化提供者与 VM 服务
	provider, err := adapters.NewVirtualizationProvider(envOr("VIRT_PROVIDER", "mock"), map[string]string{
		"url":      envOr("VIRT_URL", envOr("VIRT_ESXI_URL", envOr("VIRT_WS_URL", ""))),
		"username": envOr("VIRT_USERNAME", envOr("VIRT_ESXI_USERNAME", envOr("VIRT_WS_USERNAME", ""))),
		"password": envOr("VIRT_PASSWORD", envOr("VIRT_ESXI_PASSWORD", envOr("VIRT_WS_PASSWORD", ""))),
		"insecure": envOr("VIRT_INSECURE", envOr("VIRT_ESXI_INSECURE", envOr("VIRT_WS_INSECURE", "false"))),
	})
	if err != nil {
		log.Fatalf("虚拟化提供者初始化失败: %v", err)
	}
	vmService := services.NewVMService(vmRepo, provider)

	// 启动时从平台同步一次虚拟机目录
	if n, err := vmService.SyncVMs(ctx); err != nil {
		log.Printf("警告: 初始 VM 同步失败: %v", err)
	} else {
		log.Printf("VM 目录已同步: %d 台", n)
	}

	// 路由
	api.AppVersion = AppVersion
	guacHandler := v1.GuacHandlerForEnv(vmService.GetVM)
	router := api.NewRouter(tm, &api.HandlerSet{
		Auth:        v1.NewAuthHandler(authProvider, tm),
		VM:          v1.NewVMHandler(vmService),
		Menu:        v1.NewMenuHandler(pluginRepo),
		Plugin:      v1.NewPluginHandler(pluginRepo),
		PluginProxy: v1.NewPluginProxyHandler(pluginRepo),
		User:        v1.NewUserHandler(userRepo),
		Role:        v1.NewRoleHandler(roleRepo, roleLoader),
		Guac:        guacHandler,
		Native:      native,
		NativeDeps: plugins.Deps{
			Provider: envOr("VIRT_PROVIDER", "mock"),
			WebURL:   envOr("ESXI_WEB_URL", deriveWebURL(envOr("VIRT_URL", envOr("VIRT_ESXI_URL", "")))),
		},
		PluginRepo:  pluginRepo,
	})

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("PortalT listening on %s", listenAddr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server exited: %v", err)
	}
}

// envOr 读取环境变量，为空时返回默认值。
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// deriveWebURL 从虚拟化平台 SDK 地址推导其 Web 管理界面地址
// （如 https://esxi.lan/sdk → https://esxi.lan/ui/）；无法解析时返回空串，
// 表示该平台无内置 Web 界面（workstation/mock）。
func deriveWebURL(sdkURL string) string {
	u, err := url.Parse(strings.TrimSpace(sdkURL))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	return u.Scheme + "://" + u.Host + "/ui/"
}

// envSeconds 读取以秒为单位的环境变量，非法或空时返回默认值。
func envSeconds(key string, fallback int64) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return time.Duration(fallback) * time.Second
}
