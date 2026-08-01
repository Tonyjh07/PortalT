package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"portalt/internal/api/middleware"
	"portalt/internal/api/v1"
	"portalt/internal/ports"
)

// AppVersion 由 main 注入，用于健康检查。
var AppVersion = "v0.1"

// NewRouter 装配全部路由与中间件。
// 返回只读的 *gin.Engine，监听由调用方决定。
func NewRouter(tm ports.TokenManager, auth *v1.AuthHandler) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	// 健康检查
	router.GET("/healthz", func(c *gin.Context) {
		c.String(http.StatusOK, "PortalT %s", AppVersion)
	})

	// API v1
	v1g := router.Group("/api/v1")

	// 认证（公开）
	authG := v1g.Group("/auth")
	authG.POST("/login", auth.Login)
	authG.POST("/refresh", auth.Refresh)

	// 受保护路由
	protected := v1g.Group("")
	protected.Use(middleware.AuthRequired(tm))
	{
		authG2 := protected.Group("/auth")
		authG2.GET("/me", auth.Me)
	}

	return router
}
