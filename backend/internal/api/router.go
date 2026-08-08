package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"portalt/internal/api/middleware"
	"portalt/internal/api/response"
	"portalt/internal/api/v1"
	"portalt/internal/domain"
	"portalt/internal/plugins"
	"portalt/internal/ports"
)

// AppVersion 由 main 注入，用于健康检查。
var AppVersion = "v0.1"

// HandlerSet 路由所需的全部处理器。
type HandlerSet struct {
	Auth        *v1.AuthHandler
	VM          *v1.VMHandler
	Menu        *v1.MenuHandler
	Plugin      *v1.PluginHandler
	PluginProxy *v1.PluginProxyHandler
	User        *v1.UserHandler
	Role        *v1.RoleHandler
	Guac        v1.GuacProxy
	Platform    *v1.PlatformHandler

	// 原生插件（可选，nil 时相关路由不挂载）
	Native     *plugins.Registry
	NativeDeps plugins.Deps
	PluginRepo ports.PluginRepository

	// 权限字典与虚拟机资源授权（可选，nil 时相关能力禁用/跳过校验）
	Permissions ports.PermissionRepository
	VMAccessH   *v1.VMAccessHandler
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
	if hs.Role != nil && hs.Role.Loader() != nil {
		protected.Use(middleware.AttachPermissions(hs.Role.Loader()))
	}
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
		vms.PUT("/:id/metadata", middleware.RequirePermission(domain.PERM_VM_MANAGE), hs.VM.UpdateMetadata)

		// 动态菜单（plugin:view）
		protected.GET("/menu", middleware.RequirePermission(domain.PERM_PLUGIN_VIEW), hs.Menu.Menu)

		// 平台信息（plugin:view，前端插件页判断接入状态）
		if hs.Platform != nil {
			protected.GET("/platform", middleware.RequirePermission(domain.PERM_PLUGIN_VIEW), hs.Platform.Info)
		}

		// 插件管理（plugin:manage，管理员）
		plugins := protected.Group("/plugins", middleware.RequirePermission(domain.PERM_PLUGIN_MANAGE))
		plugins.GET("", hs.Plugin.List)
		plugins.POST("", hs.Plugin.Create)
		plugins.PUT("/:id", hs.Plugin.Update)
		plugins.DELETE("/:id", hs.Plugin.Delete)

		// 脚本插件标准 API 代理（plugin:view + 插件自身权限 + 端点白名单）
		if hs.PluginProxy != nil {
			proxy := protected.Group("/plugin-proxy", middleware.RequirePermission(domain.PERM_PLUGIN_VIEW))
			proxy.Any("/:pluginId/*path", hs.PluginProxy.Proxy)
		}

		// 原生插件 API（plugin:view + 插件启用闸门；路由由插件自身挂载）
		if hs.Native != nil && hs.PluginRepo != nil {
			nativeG := protected.Group("/plugins/native", middleware.RequirePermission(domain.PERM_PLUGIN_VIEW))
			hs.Native.MountAPI(nativeG, hs.NativeDeps, hs.PluginRepo)
			hs.Native.MountStatic(router)
		}

		// 用户管理（user:manage，管理员）
		users := protected.Group("/users", middleware.RequirePermission(domain.PERM_USER_MANAGE))
		users.GET("", hs.User.List)
		users.POST("", hs.User.Create)
		users.PUT("/:id", hs.User.Update)
		users.DELETE("/:id", hs.User.Delete)
		// 虚拟机资源授权分配（同 user:manage）
		if hs.VMAccessH != nil {
			users.GET("/:id/vm-access", hs.VMAccessH.Get)
			users.PUT("/:id/vm-access", hs.VMAccessH.Set)
		}

		// 角色权限（user:manage，管理员）
		roles := protected.Group("/roles", middleware.RequirePermission(domain.PERM_USER_MANAGE))
		roles.GET("", hs.Role.List)
		roles.GET("/permissions", hs.Role.Permissions)
		roles.POST("", hs.Role.Create)
		roles.PUT("/:id", hs.Role.Update)
		roles.DELETE("/:id", hs.Role.Delete)

		// Guacamole 远程桌面代理（vm:console + 资源级授权）
		if hs.Guac != nil {
			protected.GET("/guac/ws/:vmId", middleware.RequirePermission(domain.PERM_VM_CONSOLE), hs.Guac.Proxy)
		} else {
			protected.GET("/guac/ws/:vmId", middleware.RequirePermission(domain.PERM_VM_CONSOLE), func(c *gin.Context) {
				response.Error(c, http.StatusServiceUnavailable, response.CodeServerError, "Guacamole 未配置")
			})
		}
	}

	return router
}
