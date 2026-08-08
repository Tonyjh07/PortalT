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
		ID: "p1", Name: "脚本工具", Type: domain.PluginTypeAccess, IsActive: true,
		Endpoints: []domain.PluginEndpoint{{Method: "GET", Path: "/api/info", Name: "信息"}},
	}
	r, _ := setupProxy(t, plugin, func(w http.ResponseWriter, req *http.Request) {
		seen["user"] = req.Header.Get("X-PortalT-User")
		seen["role"] = req.Header.Get("X-PortalT-Role")
		seen["perms"] = req.Header.Get("X-PortalT-Perms")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hello":"world"}`))
	})

	w := proxyDo(t, r, http.MethodGet, "/proxy/p1/api/info")
	t.Logf("DEBUG body=%s", w.Body.String())
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "alice", seen["user"])
	assert.Equal(t, "admin", seen["role"])
	// X-PortalT-Perms：admin 内置表权限集合的 JSON（排序确定）
	var got []string
	require.NoError(t, json.Unmarshal([]byte(seen["perms"]), &got))
	assert.Contains(t, got, "vm:manage")
	assert.Equal(t, `{"hello":"world"}`, w.Body.String())
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
}

func TestPluginProxy_HeaderPerms_UsesRuntimeSet(t *testing.T) {
	// 装载角色矩阵时，X-PortalT-Perms 取矩阵内容（单一事实来源）
	seen := make(chan string, 1)
	plugin := &domain.Plugin{
		ID: "p1", Name: "脚本工具", Type: domain.PluginTypeAccess, IsActive: true,
		Endpoints: []domain.PluginEndpoint{{Method: "GET", Path: "/api/info"}},
	}
	repo := memory.NewPluginRepository()
	require.NoError(t, repo.Save(plugin))
	h := NewPluginProxyHandler(repo)
	user := &domain.User{ID: "u1", Username: "alice", Role: domain.RoleAdmin}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("auth.user", user)
		c.Set("auth.perms", map[string]struct{}{"vm:view": {}, "vm:console": {}})
		c.Next()
	})
	r.Any("/proxy/:pluginId/*path", h.Proxy)

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		seen <- req.Header.Get("X-PortalT-Perms")
	}))
	defer up.Close()
	plugin.ApiURL = up.URL

	w := proxyDo(t, r, http.MethodGet, "/proxy/p1/api/info")
	assert.Equal(t, http.StatusOK, w.Code)
	perms := <-seen
	assert.Equal(t, `["vm:console","vm:view"]`, perms)
}

func TestPluginProxy_EndpointWhitelist(t *testing.T) {
	plugin := &domain.Plugin{
		ID: "p1", Name: "脚本工具", Type: domain.PluginTypeAccess, IsActive: true,
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
		ID: "p1", Name: "停用", Type: domain.PluginTypeAccess, IsActive: true,
		Endpoints: []domain.PluginEndpoint{{Method: "GET", Path: "/api/info"}},
	}
	r, _ := setupProxy(t, plugin, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	plugin.IsActive = false
	w := proxyDo(t, r, http.MethodGet, "/proxy/p1/api/info")
	assert.Equal(t, http.StatusForbidden, w.Code)

	native := &domain.Plugin{ID: "p2", Name: "原生", Type: domain.PluginTypeNative, IsActive: true}
	repo := memory.NewPluginRepository()
	require.NoError(t, repo.Save(native))
	h := NewPluginProxyHandler(repo)
	r2 := gin.New()
	r2.Use(func(c *gin.Context) { c.Set("auth.user", &domain.User{Role: domain.RoleAdmin}); c.Next() })
	r2.Any("/proxy/:pluginId/*path", h.Proxy)
	w = proxyDo(t, r2, http.MethodGet, "/proxy/p2/api/info")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPluginProxy_PermissionAndUnreachable(t *testing.T) {
	plugin := &domain.Plugin{
		ID: "p1", Name: "受限", Type: domain.PluginTypeAccess, IsActive: true,
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
		ID: "p2", Name: "离线", Type: domain.PluginTypeAccess, IsActive: true,
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
