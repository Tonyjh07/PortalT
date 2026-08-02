package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"portalt/internal/api/middleware"
	"portalt/internal/api/response"
	"portalt/internal/api/v1"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

// AppVersion 由 main 注入，用于健康检查。
var AppVersion = "v0.1"

// HandlerSet 路由所需的全部处理器。
type HandlerSet struct {
	Auth   *v1.AuthHandler
	VM     *v1.VMHandler
	Menu   *v1.MenuHandler
	Plugin *v1.PluginHandler
	Guac   v1.GuacProxy
}

// NewRouter 装配全部路由与中间件。
// 返回只读的 *gin.Engine，监听由调用方决定。
func NewRouter(tm ports.TokenManager, hs *HandlerSet) *gin.Engine {
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
	authG.POST("/login", hs.Auth.Login)
	authG.POST("/refresh", hs.Auth.Refresh)

	// 受保护路由
	protected := v1g.Group("")
	protected.Use(middleware.AuthRequired(tm))
	{
		authG2 := protected.Group("/auth")
		authG2.GET("/me", hs.Auth.Me)

		// 虚拟机（vm:view）
		vms := protected.Group("/vms", middleware.RequirePermission(domain.PERM_VM_VIEW))
		vms.GET("", hs.VM.List)
		vms.GET("/:id", hs.VM.Get)
		vms.GET("/:id/status", hs.VM.Status)
		vms.POST("/:id/start", middleware.RequirePermission(domain.PERM_VM_START), hs.VM.Start)
		vms.POST("/:id/stop", middleware.RequirePermission(domain.PERM_VM_STOP), hs.VM.Stop)
		vms.POST("/:id/restart", middleware.RequirePermission(domain.PERM_VM_RESTART), hs.VM.Restart)

		// 动态菜单（plugin:view）
		protected.GET("/menu", middleware.RequirePermission(domain.PERM_PLUGIN_VIEW), hs.Menu.Menu)

		// 插件管理（plugin:manage，管理员）
		plugins := protected.Group("/plugins", middleware.RequirePermission(domain.PERM_PLUGIN_MANAGE))
		plugins.GET("", hs.Plugin.List)
		plugins.POST("", hs.Plugin.Create)
		plugins.PUT("/:id", hs.Plugin.Update)
		plugins.DELETE("/:id", hs.Plugin.Delete)

		// Guacamole 远程桌面代理（vm:view）
		if hs.Guac != nil {
			protected.GET("/guac/ws/:vmId", middleware.RequirePermission(domain.PERM_VM_VIEW), hs.Guac.Proxy)
		} else {
			protected.GET("/guac/ws/:vmId", middleware.RequirePermission(domain.PERM_VM_VIEW), func(c *gin.Context) {
				response.Error(c, http.StatusServiceUnavailable, response.CodeServerError, "Guacamole 未配置")
			})
		}
	}

	return router
}
