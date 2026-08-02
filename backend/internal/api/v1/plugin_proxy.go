package v1

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"portalt/internal/api/middleware"
	"portalt/internal/api/response"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

// PluginProxyHandler 脚本插件标准 API 代理处理器。
// 把门户鉴权后的请求转发到插件的 API 服务（白名单端点），
// 并透传调用者身份（X-PortalT-User / X-PortalT-Role），供插件侧做二次鉴权。
type PluginProxyHandler struct {
	plugins ports.PluginRepository
	client  *http.Client
}

// NewPluginProxyHandler 创建代理处理器。
func NewPluginProxyHandler(plugins ports.PluginRepository) *PluginProxyHandler {
	return &PluginProxyHandler{
		plugins: plugins,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Proxy GET/POST/PUT/DELETE /api/v1/plugin-proxy/:pluginId/*path
// 转发到插件 ApiURL + 请求路径。
// 流程：插件必须启用 → 当前用户须具备插件声明权限 → 端点必须在白名单内。
func (h *PluginProxyHandler) Proxy(c *gin.Context) {
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
	if !plugin.IsEnabled() {
		response.Error(c, http.StatusForbidden, response.CodeForbidden, "插件已停用")
		return
	}
	if domain.NormalizePluginType(plugin.Type) != domain.PluginTypeProxy {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, "该插件不是代理类型")
		return
	}
	user := middleware.CurrentUser(c)
	if !plugin.CanAccess(user, middleware.CurrentPerms(c)) {
		response.Error(c, http.StatusForbidden, response.CodeForbidden, "权限不足")
		return
	}

	// 白名单端点校验：方法与路径必须精确匹配声明（路径归一化为无前导斜杠）
	path := strings.TrimPrefix(c.Param("path"), "/")
	if path == "" {
		path = "/"
	}
	if _, ok := plugin.FindEndpoint(c.Request.Method, path); !ok {
		response.Error(c, http.StatusForbidden, response.CodeForbidden, "端点不在插件白名单内: "+c.Request.Method+" "+path)
		return
	}

	// 转发：保留 query、body 与内容类型；注入调用者身份
	target := strings.TrimSuffix(plugin.ApiURL, "/") + "/" + path
	if c.Request.URL.RawQuery != "" {
		target += "?" + c.Request.URL.RawQuery
	}
	upstream, err := http.NewRequest(c.Request.Method, target, c.Request.Body)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "构建转发请求失败")
		return
	}
	upstream.Header.Set("Content-Type", c.GetHeader("Content-Type"))
	upstream.Header.Set("X-PortalT-User", user.Username)
	upstream.Header.Set("X-PortalT-Role", string(user.Role))

	resp, err := h.client.Do(upstream)
	if err != nil {
		response.Error(c, http.StatusBadGateway, response.CodeServerError, "插件服务不可达: "+plugin.Name)
		return
	}
	defer resp.Body.Close()

	// 透传插件响应（状态码 + 头 + body），不做 envelope 包装，便于脚本工具直接使用
	for k := range resp.Header {
		for _, v := range resp.Header.Values(k) {
			c.Writer.Header().Add(k, v)
		}
	}
	c.Writer.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	c.Writer.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(c.Writer, resp.Body)
}

// isURLSafe 校验插件地址为 http/https（防 SSRF 注入 scheme）。
func isURLSafe(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https")
}
