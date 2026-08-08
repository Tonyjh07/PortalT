package v1

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"portalt/internal/api/middleware"
	"portalt/internal/api/response"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

// NativeProxyHandler native 插件 HTTP 数据面反代处理器。
// 依据 spike 结论（方案 A：前缀占位路由 + manager 内部分发），一条通配路由
// /plugins/native/:pluginId/*path（API）与 /native/:pluginId/*path（静态）承载
// 任意插件；处理器经 ports.NativeHost 查插件回环 HTTP 地址后反代，运行时
// 无需重注册 gin 路由，天然支持热加载。
//
// 反代前必须校验：
//   - 插件存在且已启用（API 路径，静态路径仅要求运行中）
//   - 当前用户具备插件声明权限（API 路径，nativeGate 语义）
//   - 目标地址来自宿主（回环 127.0.0.1），不信任客户端提供的任何主机
type NativeProxyHandler struct {
	plugins ports.PluginRepository
	host    ports.NativeHost
}

// NewNativeProxyHandler 创建 native 反代处理器。
func NewNativeProxyHandler(plugins ports.PluginRepository, host ports.NativeHost) *NativeProxyHandler {
	return &NativeProxyHandler{plugins: plugins, host: host}
}

// APIProxy GET/POST/PUT/DELETE /api/v1/plugins/native/:pluginId/*path
// 反代插件 API 数据面：三层权限校验（plugin:view 已在路由层）→ 插件启用闸门 →
// 声明权限硬校验 → 回环地址反代，注入调用者身份头。
func (h *NativeProxyHandler) APIProxy(c *gin.Context) {
	pluginID := c.Param("pluginId")
	plugin, err := h.plugins.FindByID(pluginID)
	if err != nil {
		if errors.Is(err, ports.ErrNotFound) {
			response.Error(c, http.StatusNotFound, response.CodeNotFound, "插件不存在")
			return
		}
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "查询插件失败")
		return
	}
	if domain.NormalizePluginType(plugin.Type) != domain.PluginTypeNative {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, "该插件不是 native 类型")
		return
	}
	if !plugin.IsEnabled() {
		response.Error(c, http.StatusForbidden, response.CodeForbidden, "插件已停用")
		return
	}
	user := middleware.CurrentUser(c)
	if !plugin.CanAccess(user, middleware.CurrentPerms(c)) {
		response.Error(c, http.StatusForbidden, response.CodeForbidden, "权限不足")
		return
	}
	h.proxy(c, pluginID, true, user)
}

// StaticProxy GET /native/:pluginId/*path
// 反代插件静态前端（公开：仅承载插件静态资源，不含敏感数据）。
// 为防把插件 API 端点无鉴权暴露到公网，/api/* 一律拒绝（数据必须走鉴权 API 路径）。
// 仅要求插件运行中（进程已启动）。
func (h *NativeProxyHandler) StaticProxy(c *gin.Context) {
	if strings.HasPrefix(c.Param("path"), "/api/") {
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "API 请通过 /api/v1/plugins/native/<id>/ 访问")
		return
	}
	pluginID := c.Param("pluginId")
	h.proxy(c, pluginID, false, nil)
}

// proxy 反向代理到插件回环 HTTP 地址。identity=true 时注入调用者身份头。
// 目标 path 即通配路由捕获的 *path（含前导斜杠，插件按自身路径约定分发）。
func (h *NativeProxyHandler) proxy(c *gin.Context, pluginID string, identity bool, user *domain.User) {
	addr := h.host.HTTPAddress(pluginID)
	if addr == "" {
		response.Error(c, http.StatusServiceUnavailable, response.CodeServerError, "插件未运行或已停止")
		return
	}
	target := "http://" + addr
	tu, err := url.Parse(target)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "构建插件地址失败")
		return
	}

	path := c.Param("path")
	if path == "" {
		path = "/"
	}
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(tu)
			pr.Out.URL.Path = path
			pr.Out.URL.RawQuery = c.Request.URL.RawQuery
			// 剥离客户端伪造的身份头，防注入
			for k := range pr.Out.Header {
				if strings.HasPrefix(strings.ToLower(k), "x-portalt-") {
					pr.Out.Header.Del(k)
				}
			}
			if identity && user != nil {
				pr.Out.Header.Set("X-PortalT-User", user.Username)
				pr.Out.Header.Set("X-PortalT-Role", string(user.Role))
				perms, _ := json.Marshal(currentPermsList(c))
				pr.Out.Header.Set("X-PortalT-Perms", string(perms))
			}
		},
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "插件服务不可达", http.StatusBadGateway)
		},
	}
	proxy.ServeHTTP(c.Writer, c.Request)
}
