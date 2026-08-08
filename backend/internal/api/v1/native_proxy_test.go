package v1

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/adapters/memory"
	"portalt/internal/api/middleware"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

// addrStubHost 可编程 native 宿主桩：按插件返回回环地址（可指向 httptest 上游）。
type addrStubHost struct {
	mu         sync.Mutex
	address    string // 所有插件共用一个回环地址（测试简化）
	status     string
	noUpstream bool // true 时不创建上游服务（模拟插件未运行）
}

func (s *addrStubHost) Enable(_ context.Context, _ string) error  { return nil }
func (s *addrStubHost) Disable(_ context.Context, _ string) error { return nil }
func (s *addrStubHost) Restart(_ context.Context, _ string) error { return nil }
func (s *addrStubHost) Status(_ string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

func (s *addrStubHost) HTTPAddress(_ string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.address
}

// setupNativeProxy 组装 native 反代路由（API 走鉴权 + plugin:view，静态公开）。
func setupNativeProxy(t *testing.T, plugin *domain.Plugin, host ports.NativeHost) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	if ah, ok := host.(*addrStubHost); ok && !ah.noUpstream {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Upstream-Echo", r.Header.Get("X-PortalT-User"))
			_, _ = w.Write([]byte("up:" + r.URL.Path))
		}))
		t.Cleanup(srv.Close)
		ah.mu.Lock()
		ah.address = srv.Listener.Addr().String()
		ah.mu.Unlock()
	}

	repo := memory.NewPluginRepository()
	require.NoError(t, repo.Save(plugin))
	h := NewNativeProxyHandler(repo, host)

	user := &domain.User{ID: "u1", Username: "alice", Role: domain.RoleAdmin}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("auth.user", user)
		c.Set("auth.perms", map[string]struct{}{domain.PERM_PLUGIN_VIEW: {}})
		c.Next()
	})
	api := r.Group("/api/v1/plugins/native", middleware.RequirePermission(domain.PERM_PLUGIN_VIEW))
	api.Any("/:pluginId/*path", h.APIProxy)
	r.Any("/native/:pluginId/*path", h.StaticProxy)
	return r
}

// notifyRecorder 包装 httptest.ResponseRecorder 并实现 http.CloseNotifier：
// Go 1.26 起 ResponseRecorder 不再实现 CloseNotify，而 gin 的 responseWriter
// 对该接口做非安全断言，ReverseProxy 在测试中触发会 panic。nil 通道等价于不监听。
type notifyRecorder struct {
	*httptest.ResponseRecorder
}

func (n *notifyRecorder) CloseNotify() <-chan bool { return nil }

func nativeProxyDo(t *testing.T, r *gin.Engine, method, path string, setHeader func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if setHeader != nil {
		setHeader(req)
	}
	rec := &notifyRecorder{ResponseRecorder: httptest.NewRecorder()}
	r.ServeHTTP(rec, req)
	return rec.ResponseRecorder
}

// TestNativeProxy_API_InjectsIdentity_StripsForgery API 路径注入身份头并剥离客户端伪造头。
func TestNativeProxy_API_InjectsIdentity_StripsForgery(t *testing.T) {
	plugin := &domain.Plugin{
		ID: "hello", Name: "Hello", Type: domain.PluginTypeNative, IsActive: true,
		Permission: "plugin:view", Status: "running",
	}
	host := &addrStubHost{status: "running"}
	r := setupNativeProxy(t, plugin, host)

	w := nativeProxyDo(t, r, http.MethodGet, "/api/v1/plugins/native/hello/api/hello", func(req *http.Request) {
		req.Header.Set("X-PortalT-User", "eve") // 伪造身份头，应被剥离后由服务端注入
	})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "alice", w.Header().Get("X-Upstream-Echo"))
	assert.Equal(t, "up:/api/hello", w.Body.String())
}

// TestNativeProxy_Static_NoIdentity_AndBlocksAPI 静态路径不注入身份，且 /api/* 一律拒绝。
func TestNativeProxy_Static_NoIdentity_AndBlocksAPI(t *testing.T) {
	plugin := &domain.Plugin{
		ID: "hello", Name: "Hello", Type: domain.PluginTypeNative, IsActive: true,
		Status: "running",
	}
	host := &addrStubHost{status: "running"}
	r := setupNativeProxy(t, plugin, host)

	// 静态资源：不注入身份头（上游收到的 X-PortalT-User 应为空）
	w := nativeProxyDo(t, r, http.MethodGet, "/native/hello/static/app.js", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("X-Upstream-Echo"))
	assert.Equal(t, "up:/static/app.js", w.Body.String())

	// 静态路径请求插件 API：拒绝（数据必须走鉴权 API）
	w = nativeProxyDo(t, r, http.MethodGet, "/native/hello/api/hello", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

// TestNativeProxy_Static_StripsForgery 静态路径同样剥离客户端伪造的身份头。
func TestNativeProxy_Static_StripsForgery(t *testing.T) {
	plugin := &domain.Plugin{
		ID: "hello", Name: "Hello", Type: domain.PluginTypeNative, IsActive: true,
		Status: "running",
	}
	host := &addrStubHost{status: "running"}
	r := setupNativeProxy(t, plugin, host)

	w := nativeProxyDo(t, r, http.MethodGet, "/native/hello/app.js", func(req *http.Request) {
		req.Header.Set("X-PortalT-User", "eve")
		req.Header.Set("X-PortalT-Role", "admin")
	})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, w.Header().Get("X-Upstream-Echo"), "伪造身份头必须被剥离，不得透传")
}

// TestNativeProxy_API_PermissionGate API 路径做声明权限硬校验。
func TestNativeProxy_API_PermissionGate(t *testing.T) {
	plugin := &domain.Plugin{
		ID: "hello", Name: "Hello", Type: domain.PluginTypeNative, IsActive: true,
		Permission: "user:manage", // 当前测试用户无此权限
		Status:     "running",
	}
	host := &addrStubHost{status: "running"}
	r := setupNativeProxy(t, plugin, host)

	w := nativeProxyDo(t, r, http.MethodGet, "/api/v1/plugins/native/hello/api/hello", nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// TestNativeProxy_API_DisabledPlugin 已停用插件 API 请求被拒。
func TestNativeProxy_API_DisabledPlugin(t *testing.T) {
	plugin := &domain.Plugin{
		ID: "hello", Name: "Hello", Type: domain.PluginTypeNative, IsActive: false,
		Status: "stopped",
	}
	host := &addrStubHost{status: "stopped"}
	r := setupNativeProxy(t, plugin, host)

	w := nativeProxyDo(t, r, http.MethodGet, "/api/v1/plugins/native/hello/api/hello", nil)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

// TestNativeProxy_API_NotRunning 插件未运行（地址为空）→ 503。
func TestNativeProxy_API_NotRunning(t *testing.T) {
	plugin := &domain.Plugin{
		ID: "hello", Name: "Hello", Type: domain.PluginTypeNative, IsActive: true,
		Status: "stopped",
	}
	host := &addrStubHost{status: "stopped", address: "", noUpstream: true}
	r := setupNativeProxy(t, plugin, host)

	w := nativeProxyDo(t, r, http.MethodGet, "/api/v1/plugins/native/hello/api/hello", nil)
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

// TestNativeProxy_API_NotFound 插件不存在 → 404。
func TestNativeProxy_API_NotFound(t *testing.T) {
	plugin := &domain.Plugin{ID: "hello", Name: "Hello", Type: domain.PluginTypeNative, IsActive: true}
	host := &addrStubHost{status: "running"}
	r := setupNativeProxy(t, plugin, host)

	w := nativeProxyDo(t, r, http.MethodGet, "/api/v1/plugins/native/nope/api/hello", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
