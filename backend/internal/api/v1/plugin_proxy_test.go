package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/adapters/memory"
	"portalt/internal/api/middleware"
	"portalt/internal/domain"
)

// setupProxy 组装代理路由 + 假插件 API 服务。
func setupProxy(t *testing.T, plugin *domain.Plugin, upstream http.HandlerFunc) (*gin.Engine, string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var upURL string
	if upstream != nil {
		upSrv := httptest.NewServer(upstream)
		upURL = upSrv.URL
		plugin.ApiURL = upSrv.URL
		t.Cleanup(upSrv.Close)
	}

	repo := memory.NewPluginRepository()
	require.NoError(t, repo.Save(plugin))
	h := NewPluginProxyHandler(repo)

	user := &domain.User{ID: "u1", Username: "alice", Role: domain.RoleAdmin}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("auth.user", user); c.Next() })
	r.Any("/proxy/:pluginId/*path", h.Proxy)
	t.Logf("DEBUG after setup apiURL=%q endpoints=%v", plugin.ApiURL, plugin.Endpoints)
	return r, upURL
}

func TestPluginProxy_ForwardAndIdentity(t *testing.T) {
	seen := make(map[string]string)
	plugin := &domain.Plugin{
		ID: "p1", Name: "脚本工具", Type: domain.PluginTypeProxy, IsActive: true,
		Endpoints: []domain.PluginEndpoint{{Method: "GET", Path: "/api/info", Name: "信息"}},
	}
	r, _ := setupProxy(t, plugin, func(w http.ResponseWriter, req *http.Request) {
		seen["user"] = req.Header.Get("X-PortalT-User")
		seen["role"] = req.Header.Get("X-PortalT-Role")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hello":"world"}`))
	})

	w := proxyDo(t, r, http.MethodGet, "/proxy/p1/api/info")
	t.Logf("DEBUG body=%s", w.Body.String())
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "alice", seen["user"])
	assert.Equal(t, "admin", seen["role"])
	assert.Equal(t, `{"hello":"world"}`, w.Body.String())
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
}

func TestPluginProxy_EndpointWhitelist(t *testing.T) {
	plugin := &domain.Plugin{
		ID: "p1", Name: "脚本工具", Type: domain.PluginTypeProxy, IsActive: true,
		Endpoints: []domain.PluginEndpoint{{Method: "GET", Path: "/api/info"}},
	}
	r, _ := setupProxy(t, plugin, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	// 白名单外路径 → 403
	w := proxyDo(t, r, http.MethodGet, "/proxy/p1/api/secret")
	assert.Equal(t, http.StatusForbidden, w.Code)
	// 白名单内方法匹配
	w = proxyDo(t, r, http.MethodGet, "/proxy/p1/api/info")
	assert.Equal(t, http.StatusOK, w.Code)
	// 方法不匹配 → 403
	w = proxyDo(t, r, http.MethodPost, "/proxy/p1/api/info")
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestPluginProxy_DisabledAndWrongType(t *testing.T) {
	plugin := &domain.Plugin{
		ID: "p1", Name: "停用", Type: domain.PluginTypeProxy, IsActive: true,
		Endpoints: []domain.PluginEndpoint{{Method: "GET", Path: "/api/info"}},
	}
	r, _ := setupProxy(t, plugin, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	plugin.IsActive = false
	w := proxyDo(t, r, http.MethodGet, "/proxy/p1/api/info")
	assert.Equal(t, http.StatusForbidden, w.Code)

	iframe := &domain.Plugin{ID: "p2", Name: "嵌入", Type: domain.PluginTypeIframe, IsActive: true}
	repo := memory.NewPluginRepository()
	require.NoError(t, repo.Save(iframe))
	h := NewPluginProxyHandler(repo)
	r2 := gin.New()
	r2.Use(func(c *gin.Context) { c.Set("auth.user", &domain.User{Role: domain.RoleAdmin}); c.Next() })
	r2.Any("/proxy/:pluginId/*path", h.Proxy)
	w = proxyDo(t, r2, http.MethodGet, "/proxy/p2/api/info")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPluginProxy_PermissionAndUnreachable(t *testing.T) {
	plugin := &domain.Plugin{
		ID: "p1", Name: "受限", Type: domain.PluginTypeProxy, IsActive: true,
		Permission: "plugin:view",
		Endpoints:  []domain.PluginEndpoint{{Method: "GET", Path: "/api/info"}},
	}
	// viewer 无 plugin:view → 403
	repo := memory.NewPluginRepository()
	require.NoError(t, repo.Save(plugin))
	h := NewPluginProxyHandler(repo)
	user := &domain.User{ID: "u1", Username: "v", Role: domain.RoleViewer}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("auth.user", user); c.Next() })
	r.Any("/proxy/:pluginId/*path", h.Proxy)
	w := proxyDo(t, r, http.MethodGet, "/proxy/p1/api/info")
	assert.Equal(t, http.StatusForbidden, w.Code)

	// 服务不可达 → 502
	plugin2 := &domain.Plugin{
		ID: "p2", Name: "离线", Type: domain.PluginTypeProxy, IsActive: true,
		ApiURL:    "http://127.0.0.1:1",
		Endpoints: []domain.PluginEndpoint{{Method: "GET", Path: "/api/info"}},
	}
	repo2 := memory.NewPluginRepository()
	require.NoError(t, repo2.Save(plugin2))
	h2 := NewPluginProxyHandler(repo2)
	r2 := gin.New()
	r2.Use(func(c *gin.Context) { c.Set("auth.user", &domain.User{Role: domain.RoleAdmin}); c.Next() })
	r2.Any("/proxy/:pluginId/*path", h2.Proxy)
	w = proxyDo(t, r2, http.MethodGet, "/proxy/p2/api/info")
	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestPluginProxy_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := memory.NewPluginRepository()
	h := NewPluginProxyHandler(repo)
	r2 := gin.New()
	r2.Use(func(c *gin.Context) { c.Set("auth.user", &domain.User{Role: domain.RoleAdmin}); c.Next() })
	r2.Any("/proxy/:pluginId/*path", h.Proxy)
	w := proxyDo(t, r2, http.MethodGet, "/proxy/nope/api/info")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func proxyDo(t *testing.T, r *gin.Engine, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(`{"a":1}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

var _ = json.Marshal
var _ = middleware.AttachPermissions
