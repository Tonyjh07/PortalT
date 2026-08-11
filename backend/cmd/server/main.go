// PortalT 后端服务入口 (Phase 5)
//
// 负责依赖装配：数据库（OpenDBFromEnv）→ 仓储 → 认证/JWT →
// 管理员引导 → Gin 路由，最后启动 HTTP 服务。
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"portalt/internal/adapters"
	authadapter "portalt/internal/adapters/auth"
	"portalt/internal/adapters/gormstore"
	"portalt/internal/api"
	"portalt/internal/api/middleware"
	"portalt/internal/api/v1"
	"portalt/internal/domain"
	"portalt/internal/domain/services"
	"portalt/internal/pluginhost"
	"portalt/internal/ports"
)

const (
	// AppVersion 当前版本号
	AppVersion = "v0.1"
)

func main() {
	log.Println("PortalT", AppVersion, "starting...")

	// HTTP 监听地址：PORT 可覆盖（如 "127.0.0.1:8080"、"0.0.0.0:8080"）；
	// 若只给裸端口（如 "8080"），自动绑定回环地址，避免与前端 preview
	// 的 PORT=3001 用法混淆导致启动失败
	listenAddr := normalizeAddr(envOr("PORT", "127.0.0.1:8080"))

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

	// 权限字典引导（permissions 表；供角色编辑/插件声明校验）
	permRepo := gormstore.NewPermissionRepository(db)
	if err := services.EnsureDefaultPermissions(ctx, permRepo); err != nil {
		log.Fatalf("权限字典引导失败: %v", err)
	}
	log.Printf("权限字典已就绪")

	// 虚拟机资源级授权仓储
	vmAccessRepo := gormstore.NewVMAccessRepository(db)

	// Caddy 插件规则管理器（access 插件落盘 plugins.d/ 并 reload；
	// PLUGIN_CADDY_DIR 为空 = 本地 dev 无 Caddy，仅接受不落盘不报错）
	caddy := pluginhost.NewCaddyManager(
		envOr("PLUGIN_CADDY_DIR", ""),
		envOr("CADDY_RELOAD_CMD", ""),
	)

	// 默认 access 插件引导（esxi-admin：ESXi 管理入口 + 反代规则默认值）
	if err := seedDefaultAccessPlugins(ctx, pluginRepo); err != nil {
		log.Fatalf("默认插件引导失败: %v", err)
	}
	// 启动时把启用且含规则的 access 插件写入 plugins.d 并 reload 一次
	if all, err := pluginRepo.FindAll(); err != nil {
		log.Printf("警告: 读取插件列表失败: %v", err)
	} else if err := caddy.SyncAll(all); err != nil {
		log.Printf("警告: Caddy 插件规则同步失败: %v", err)
	}

	// 原生插件宿主：扫描 PLUGINS_DIR 下的 manifest，启动/重启 native 插件进程
	// 并回写运行态。PLUGINS_DIR 为空 = 禁用态，反代路由恒挂载但一律 503。
	pluginManager := pluginhost.NewManager(
		envOr("PLUGINS_DIR", ""),
		pluginRepo,
		AppVersion,
	)
	// manifest 声明的权限入库前校验权限字典（与 access 插件同约束）
	pluginManager.SetPermissionRepo(permRepo)
	if pluginManager.Enabled() {
		if err := pluginManager.Start(ctx); err != nil {
			log.Fatalf("原生插件宿主启动失败: %v", err)
		}
		log.Printf("原生插件宿主已启动（plugins dir: %s）", envOr("PLUGINS_DIR", ""))
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

	// 周期后台同步：保持仓储中 VM 状态新鲜（电源状态变化、平台新增/删除 VM）。
	// 间隔由 VM_SYNC_INTERVAL 控制（秒），默认 60s；避免状态只靠详情页轮询回刷，
	// 列表页读到陈旧状态。单 goroutine 串行执行，不会重叠。
	if syncInterval := envSeconds("VM_SYNC_INTERVAL", 60); syncInterval > 0 {
		go periodicVMSync(ctx, vmService, syncInterval)
	}

	// 路由
	api.AppVersion = AppVersion
	guacHandler := v1.GuacHandlerForEnv(vmService.GetVM, vmAccessRepo)
	router := api.NewRouter(tm, &api.HandlerSet{
		Auth:        v1.NewAuthHandler(authProvider, tm, roleLoader),
		VM:          v1.NewVMHandler(vmService, vmAccessRepo),
		Menu:        v1.NewMenuHandler(pluginRepo),
		Plugin:      v1.NewPluginHandler(pluginRepo, permRepo, caddy, pluginManager),
		PluginProxy: v1.NewPluginProxyHandler(pluginRepo),
		User:        v1.NewUserHandler(userRepo, roleRepo, vmAccessRepo),
		Role:        v1.NewRoleHandler(roleRepo, permRepo, roleLoader),
		VMAccessH:   v1.NewVMAccessHandler(vmAccessRepo),
		Guac:        guacHandler,
		Platform: v1.NewPlatformHandler(
			envOr("VIRT_PROVIDER", "mock"),
			envOr("ESXI_WEB_URL", deriveWebURL(envOr("VIRT_URL", envOr("VIRT_ESXI_URL", "")))),
		),
		NativeProxy: v1.NewNativeProxyHandler(pluginRepo, pluginManager),
		PluginRepo:  pluginRepo,
		Permissions: permRepo,
	})

	srv := &http.Server{
		Addr:              listenAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("PortalT listening on %s", listenAddr)

	// 优雅停机：等待 OS 信号（Ctrl+C / systemctl stop），
	// 停止 HTTP 服务并逐个优雅停止插件进程（Shutdown RPC + kill 兜底），
	// 避免 Linux 上停止服务后残留孤儿插件子进程。
	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-sigCtx.Done()
		log.Println("收到退出信号，开始优雅停机...")
		// 插件停机与 HTTP drain 分别计时，避免共享预算互相挤占
		pluginCtx, pcancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer pcancel()
		pluginManager.Shutdown(pluginCtx)
		httpCtx, hcancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer hcancel()
		_ = srv.Shutdown(httpCtx)
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		// 启动/运行期出错退出：同样要回收插件子进程，避免残留孤儿进程
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		pluginManager.Shutdown(shutdownCtx)
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

// periodicVMSync 周期同步 VM 目录：单 goroutine 串行执行（防重叠），
// 失败仅告警不中断；响应 ctx 取消以便将来优雅停机。
func periodicVMSync(ctx context.Context, svc *services.VMService, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := svc.SyncVMs(ctx); err != nil {
				log.Printf("警告: VM 周期同步失败: %v", err)
			}
		}
	}
}

// normalizeAddr 若地址不含 ":"（视为裸端口）则补全为 "127.0.0.1:<port>"。
func normalizeAddr(addr string) string {
	if !strings.Contains(addr, ":") {
		return "127.0.0.1:" + addr
	}
	return addr
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

// seedDefaultAccessPlugins 幂等引导默认 access 插件（esxi-admin）。
// 已存在时：
//   - 类型为 access 且 CaddyRules 为空 → 回填默认反代规则（管理员配置优先，不覆盖）；
//   - CaddyRules 归一化后等于旧默认（DefaultESXIAdminCaddyRulesV1/V2，即历史无鉴权或
//     缺 /ha-nfc 的默认）→ 升级为当前默认规则（逐行去空白/统一换行后比较，容忍 CRLF 与
//     textarea 保存差异；仅匹配旧默认形态才覆盖，保留管理员自定义）；
//   - CaddyRules 形似旧版 ESXi 反代（引用 {env.ESXI_UPSTREAM}）但未含鉴权闸口、又无法
//     归一化匹配 → 打告警提示手动升级，避免无鉴权暴露被静默保留；
// 记录不存在则创建。
func seedDefaultAccessPlugins(ctx context.Context, repo ports.PluginRepository) error {
	existing, err := repo.FindByID("esxi-admin")
	if errors.Is(err, ports.ErrNotFound) {
		return repo.Save(&domain.Plugin{
			ID:         "esxi-admin",
			Name:       "ESXi 管理",
			Icon:       "mdi:server-network",
			Route:      "/esxi-admin",
			Type:       domain.PluginTypeAccess,
			IframeURL:  "/esxi/ui/",
			CaddyRules: pluginhost.DefaultESXIAdminCaddyRules,
			Permission: domain.PERM_ESXI_ADMIN_USE,
			SortOrder:  90,
			IsActive:   true,
		})
	}
	if err != nil {
		return err
	}
	if domain.NormalizePluginType(existing.Type) == domain.PluginTypeAccess {
		if existing.CaddyRules == "" {
			existing.CaddyRules = pluginhost.DefaultESXIAdminCaddyRules
			return repo.Save(existing)
		}
		if norm := normalizeRules(existing.CaddyRules); norm == normalizeRules(pluginhost.DefaultESXIAdminCaddyRulesV1) ||
			norm == normalizeRules(pluginhost.DefaultESXIAdminCaddyRulesV2) {
			existing.CaddyRules = pluginhost.DefaultESXIAdminCaddyRules
			return repo.Save(existing)
		}
		if strings.Contains(existing.CaddyRules, "{env.ESXI_UPSTREAM}") &&
			!strings.Contains(existing.CaddyRules, "/api/v1/auth/gate") {
			log.Printf("[seed] esxi-admin 插件 CaddyRules 疑似旧版无鉴权规则且与默认不匹配，" +
				"未自动升级；请在插件管理页手动保存规则以接入鉴权闸口")
		}
	}
	return nil
}

// normalizeRules 归一化 Caddy 规则串用于语义比较：逐行去除首尾空白并统一换行，
// 容忍 CRLF/LF 差异与 textarea 保存时引入的空白，同时保留行内有效的指令缩进结构。
func normalizeRules(s string) string {
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimSpace(ln)
	}
	return strings.Join(lines, "\n")
}
