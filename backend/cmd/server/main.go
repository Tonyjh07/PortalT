// PortalT 后端服务入口 (Phase 5)
//
// 负责依赖装配：数据库（OpenDBFromEnv）→ 仓储 → 认证/JWT →
// 管理员引导 → Gin 路由，最后启动 HTTP 服务。
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"portalt/internal/adapters"
	authadapter "portalt/internal/adapters/auth"
	"portalt/internal/adapters/gormstore"
	"portalt/internal/api"
	"portalt/internal/api/v1"
)

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

	// 路由
	api.AppVersion = AppVersion
	router := api.NewRouter(tm, v1.NewAuthHandler(authProvider, tm))

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

// envSeconds 读取以秒为单位的环境变量，非法或空时返回默认值。
func envSeconds(key string, fallback int64) time.Duration {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return time.Duration(fallback) * time.Second
}
