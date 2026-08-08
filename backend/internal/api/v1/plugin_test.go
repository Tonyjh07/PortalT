package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/adapters/memory"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

// setupPlugin 组装插件管理路由。
func setupPlugin(t *testing.T) (*gin.Engine, *memory.PluginRepository) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := memory.NewPluginRepository()
	handler := NewPluginHandler(repo, nil, nil, nil)

	r := gin.New()
	r.POST("/plugins", handler.Create)
	r.PUT("/plugins/:id", handler.Update)
	r.DELETE("/plugins/:id", handler.Delete)
	r.GET("/plugins", handler.List)
	return r, repo
}

// stubCaddy 可编程的 Caddy 应用器桩，记录调用并模拟失败。
type stubCaddy struct {
	applied  map[string]string
	removed  []string
	reloads  int
	applyErr error
	relErr   error
}

// stubNativeHost 可编程的 native 宿主桩，记录生命周期调用并模拟失败。
type stubNativeHost struct {
	enabled   []string
	disabled  []string
	restarted []string
	err       error
}

func (s *stubNativeHost) Enable(_ context.Context, id string) error {
	if s.err != nil {
		return s.err
	}
	s.enabled = append(s.enabled, id)
	return nil
}

func (s *stubNativeHost) Disable(_ context.Context, id string) error {
	if s.err != nil {
		return s.err
	}
	s.disabled = append(s.disabled, id)
	return nil
}

func (s *stubNativeHost) Restart(_ context.Context, id string) error {
	if s.err != nil {
		return s.err
	}
	s.restarted = append(s.restarted, id)
	return nil
}

func (s *stubNativeHost) Status(id string) string { return "" }

func (s *stubNativeHost) HTTPAddress(id string) string { return "" }

func (s *stubCaddy) Apply(id, rules string) error {
	if s.applyErr != nil {
		return s.applyErr
	}
	if s.applied == nil {
		s.applied = make(map[string]string)
	}
	s.applied[id] = rules
	return nil
}

func (s *stubCaddy) Remove(id string) error {
	s.removed = append(s.removed, id)
	return nil
}

func (s *stubCaddy) Reload() error {
	s.reloads++
	return s.relErr
}

func (s *stubCaddy) HasRuleFile(id string) bool {
	_, ok := s.applied[id]
	return ok
}

func pluginDo(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestPlugin_Create_Success(t *testing.T) {
	r, repo := setupPlugin(t)
	w := pluginDo(t, r, http.MethodPost, "/plugins", map[string]any{
		"name": "Home Assistant", "icon": "mdi:home", "route": "/ha",
		"iframe_url": "https://ha.local", "sort_order": 2, "is_active": true,
	})
	require.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	data := body["data"].(map[string]any)
	assert.NotEmpty(t, data["id"])
	assert.Equal(t, "Home Assistant", data["name"])
	assert.Equal(t, "/ha", data["route"])

	all, err := repo.FindAll()
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, data["id"], all[0].ID)
}

func TestPlugin_Create_MissingRequired(t *testing.T) {
	r, _ := setupPlugin(t)
	w := pluginDo(t, r, http.MethodPost, "/plugins", map[string]any{"name": "无路由"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPluginHandler_PermissionDictValidation(t *testing.T) {
	// 权限字典已 seed：声明字典内权限 OK，字典外 400
	gin.SetMode(gin.TestMode)
	repo := memory.NewPluginRepository()
	perms := memory.NewPermissionRepository()
	require.NoError(t, perms.EnsureDefault(domain.AllPermissions()))
	handler := NewPluginHandler(repo, perms, nil, nil)

	r := gin.New()
	r.POST("/plugins", handler.Create)

	// 字典内权限 → 200
	w := pluginDo(t, r, http.MethodPost, "/plugins", map[string]any{
		"name": "受限工具", "route": "/tool", "type": "access",
		"api_url": "http://127.0.0.1:1", "permission": "vm:view",
		"endpoints": []any{map[string]any{"method": "GET", "path": "/api/info"}},
	})
	assert.Equal(t, http.StatusOK, w.Code)

	// 字典外权限 → 400
	w = pluginDo(t, r, http.MethodPost, "/plugins", map[string]any{
		"name": "越权工具", "route": "/tool2", "type": "access",
		"api_url": "http://127.0.0.1:1", "permission": "vm:destroy",
		"endpoints": []any{map[string]any{"method": "GET", "path": "/api/info"}},
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPlugin_Update_Success(t *testing.T) {
	r, repo := setupPlugin(t)
	require.NoError(t, repo.Save(&domain.Plugin{ID: "p-1", Name: "old", Route: "/ha"}))

	w := pluginDo(t, r, http.MethodPut, "/plugins/p-1", map[string]any{
		"name": "new", "route": "/ha", "iframe_url": "https://ha.local",
		"permission": "vm:view", "is_active": false,
	})
	require.Equal(t, http.StatusOK, w.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	data := body["data"].(map[string]any)
	assert.Equal(t, "p-1", data["id"])
	assert.Equal(t, "new", data["name"])
	assert.False(t, data["is_active"].(bool))
}

func TestPlugin_Update_NotFound(t *testing.T) {
	r, _ := setupPlugin(t)
	w := pluginDo(t, r, http.MethodPut, "/plugins/ghost", map[string]any{
		"name": "x", "route": "/x",
	})
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestPlugin_Delete_Success(t *testing.T) {
	r, repo := setupPlugin(t)
	require.NoError(t, repo.Save(&domain.Plugin{ID: "p-1", Name: "x", Route: "/x"}))

	w := pluginDo(t, r, http.MethodDelete, "/plugins/p-1", nil)
	require.Equal(t, http.StatusOK, w.Code)

	_, err := repo.FindByID("p-1")
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

func TestPlugin_Delete_NotFound(t *testing.T) {
	r, _ := setupPlugin(t)
	w := pluginDo(t, r, http.MethodDelete, "/plugins/ghost", nil)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestPlugin_List(t *testing.T) {
	r, repo := setupPlugin(t)
	require.NoError(t, repo.Save(&domain.Plugin{ID: "p-1", Name: "a", Route: "/a", SortOrder: 1}))
	require.NoError(t, repo.Save(&domain.Plugin{ID: "p-2", Name: "b", Route: "/b", SortOrder: 2, IsActive: false}))

	w := pluginDo(t, r, http.MethodGet, "/plugins", nil)
	require.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Data []domain.Plugin `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data, 2)
}

func TestPlugin_List_CaddyApplied(t *testing.T) {
	c := &stubCaddy{}
	r, repo, _ := setupPluginWithCaddy(t, c)
	access := &domain.Plugin{ID: "a1", Name: "access", Route: "/a", Type: domain.PluginTypeAccess, IsActive: true}
	native := &domain.Plugin{ID: "n1", Name: "native", Route: "/n", Type: domain.PluginTypeNative, IsActive: true}
	require.NoError(t, repo.Save(access))
	require.NoError(t, repo.Save(native))

	// 先 apply 使规则文件在桩中"落盘"
	require.NoError(t, c.Apply("a1", "handle /a { respond 200 }"))

	w := pluginDo(t, r, http.MethodGet, "/plugins", nil)
	require.Equal(t, http.StatusOK, w.Code)

	var raw struct {
		Data []json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &raw))
	require.Len(t, raw.Data, 2)

	for _, item := range raw.Data {
		var base struct {
			ID           string `json:"id"`
			Type         string `json:"type"`
			CaddyApplied bool   `json:"caddy_applied"`
		}
		require.NoError(t, json.Unmarshal(item, &base))
		switch base.ID {
		case "a1":
			assert.True(t, base.CaddyApplied, "access 插件有规则文件时应为 true")
		case "n1":
			assert.False(t, base.CaddyApplied, "native 插件恒为 false")
		}
	}
}

// setupPluginWithCaddy 组装带 Caddy 桩的插件管理路由。
func setupPluginWithCaddy(t *testing.T, c *stubCaddy) (*gin.Engine, *memory.PluginRepository, *stubCaddy) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := memory.NewPluginRepository()
	handler := NewPluginHandler(repo, nil, c, nil)
	r := gin.New()
	r.POST("/plugins", handler.Create)
	r.PUT("/plugins/:id", handler.Update)
	r.GET("/plugins", handler.List)
	r.DELETE("/plugins/:id", handler.Delete)
	return r, repo, c
}

func TestPlugin_CaddySync_CreateApplyReload(t *testing.T) {
	r, repo, c := setupPluginWithCaddy(t, &stubCaddy{})
	rules := "handle /esxi/* { reverse_proxy {env.ESXI_UPSTREAM}:443 }"
	w := pluginDo(t, r, http.MethodPost, "/plugins", map[string]any{
		"id": "esxi", "name": "ESXi 管理", "route": "/esxi-admin", "type": "access",
		"iframe_url": "/esxi/ui/", "caddy_rules": rules, "is_active": true,
	})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, rules, c.applied["esxi"])
	assert.Equal(t, 1, c.reloads)

	got, err := repo.FindByID("esxi")
	require.NoError(t, err)
	assert.Equal(t, rules, got.CaddyRules)
}

func TestPlugin_CaddySync_UpdateReapplies(t *testing.T) {
	r, repo, c := setupPluginWithCaddy(t, &stubCaddy{})
	require.NoError(t, repo.Save(&domain.Plugin{
		ID: "esxi", Name: "ESXi", Route: "/esxi-admin", Type: domain.PluginTypeAccess, CaddyRules: "old",
	}))
	rules := "handle /esxi/* { respond 200 }"
	w := pluginDo(t, r, http.MethodPut, "/plugins/esxi", map[string]any{
		"name": "ESXi", "route": "/esxi-admin", "type": "access",
		"iframe_url": "/esxi/ui/", "caddy_rules": rules, "is_active": true,
	})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, rules, c.applied["esxi"])
	assert.Equal(t, 1, c.reloads)
}

func TestPlugin_CaddySync_DeleteRemoves(t *testing.T) {
	r, repo, c := setupPluginWithCaddy(t, &stubCaddy{})
	require.NoError(t, repo.Save(&domain.Plugin{ID: "esxi", Name: "ESXi", Route: "/esxi-admin"}))
	w := pluginDo(t, r, http.MethodDelete, "/plugins/esxi", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{"esxi"}, c.removed)
	assert.Equal(t, 1, c.reloads)
}

func TestPlugin_CaddySync_ReloadFailureWarns(t *testing.T) {
	c := &stubCaddy{relErr: errors.New("systemctl reload 失败")}
	r, _, _ := setupPluginWithCaddy(t, c)
	w := pluginDo(t, r, http.MethodPost, "/plugins", map[string]any{
		"id": "esxi", "name": "ESXi", "route": "/esxi-admin", "type": "access",
		"iframe_url": "/esxi/ui/", "caddy_rules": "handle /esxi/* {}", "is_active": true,
	})
	// 规则已落盘 → 200 + 警告消息
	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, body["message"].(string), "reload 失败")
	assert.Equal(t, "handle /esxi/* {}", c.applied["esxi"])
}

func TestPlugin_CaddySync_ApplyFailure500(t *testing.T) {
	c := &stubCaddy{applyErr: errors.New("磁盘只读")}
	r, _, _ := setupPluginWithCaddy(t, c)
	w := pluginDo(t, r, http.MethodPost, "/plugins", map[string]any{
		"id": "esxi", "name": "ESXi", "route": "/esxi-admin", "type": "access",
		"iframe_url": "/esxi/ui/", "caddy_rules": "handle /esxi/* {}", "is_active": true,
	})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestPlugin_CaddySync_DisableRemovesRules(t *testing.T) {
	// 停用插件（is_active=false）→ 移除其 Caddy 规则文件并 reload，不再占用反代路径
	r, repo, c := setupPluginWithCaddy(t, &stubCaddy{})
	require.NoError(t, repo.Save(&domain.Plugin{
		ID: "esxi", Name: "ESXi", Route: "/esxi-admin", Type: domain.PluginTypeAccess,
		IframeURL: "/esxi/ui/", CaddyRules: "handle /esxi/* {}", IsActive: true,
	}))
	w := pluginDo(t, r, http.MethodPut, "/plugins/esxi", map[string]any{
		"name": "ESXi", "route": "/esxi-admin", "type": "access",
		"iframe_url": "/esxi/ui/", "caddy_rules": "handle /esxi/* {}", "is_active": false,
	})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{"esxi"}, c.removed)
	assert.Len(t, c.applied, 0)
	assert.Equal(t, 1, c.reloads)
}

func TestPlugin_CaddySync_ClearRulesRemoves(t *testing.T) {
	// 清空 caddy_rules → 移除规则文件并 reload
	r, repo, c := setupPluginWithCaddy(t, &stubCaddy{})
	require.NoError(t, repo.Save(&domain.Plugin{
		ID: "esxi", Name: "ESXi", Route: "/esxi-admin", Type: domain.PluginTypeAccess, IsActive: true,
	}))
	w := pluginDo(t, r, http.MethodPut, "/plugins/esxi", map[string]any{
		"name": "ESXi", "route": "/esxi-admin", "type": "access",
		"iframe_url": "/esxi/ui/", "caddy_rules": "", "is_active": true,
	})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{"esxi"}, c.removed)
	assert.Equal(t, 1, c.reloads)
}

func TestPlugin_CaddySync_SkippedForNative(t *testing.T) {
	c := &stubCaddy{}
	r, _, _ := setupPluginWithCaddy(t, c)
	// native 插件记录由宿主按 manifest 自动注册，管理 API 不可手动创建
	w := pluginDo(t, r, http.MethodPost, "/plugins", map[string]any{
		"id": "np", "name": "原生", "route": "/np", "type": "native",
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Len(t, c.applied, 0)
	assert.Equal(t, 0, c.reloads)
}

// setupNativeRouter 组装带 native 宿主桩的插件管理路由（含重启端点）。
func setupNativeRouter(t *testing.T, host ports.NativeHost) (*gin.Engine, *memory.PluginRepository, *stubNativeHost) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := memory.NewPluginRepository()
	handler := NewPluginHandler(repo, nil, nil, host)
	r := gin.New()
	r.PUT("/plugins/:id", handler.Update)
	r.DELETE("/plugins/:id", handler.Delete)
	r.POST("/plugins/:id/restart", handler.Restart)
	stub, _ := host.(*stubNativeHost)
	return r, repo, stub
}

func TestPlugin_Native_UpdateEnableDisable(t *testing.T) {
	host := &stubNativeHost{}
	r, repo, _ := setupNativeRouter(t, host)
	require.NoError(t, repo.Save(&domain.Plugin{
		ID: "hello", Name: "Hello", Route: "/hello", Type: domain.PluginTypeNative,
		IsActive: false, Status: "stopped", Permission: "vm:view",
	}))

	// 启用 → 宿主 Enable 被调用，is_active 更新
	w := pluginDo(t, r, http.MethodPut, "/plugins/hello", map[string]any{
		"name": "被忽略", "route": "/被忽略", "type": "access",
		"is_active": true,
	})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{"hello"}, host.enabled)
	assert.Empty(t, host.disabled)

	got, err := repo.FindByID("hello")
	require.NoError(t, err)
	assert.True(t, got.IsActive)
	assert.Equal(t, "vm:view", got.Permission, "未提供 permission 时保留现有声明权限")
	assert.Equal(t, "Hello", got.Name, "native 其余字段以 manifest 为准，不受请求体影响")

	// 停用 → 宿主 Disable 被调用
	w = pluginDo(t, r, http.MethodPut, "/plugins/hello", map[string]any{"is_active": false})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{"hello"}, host.disabled)
}

func TestPlugin_Native_UpdatePermission(t *testing.T) {
	host := &stubNativeHost{}
	r, repo, _ := setupNativeRouter(t, host)
	require.NoError(t, repo.Save(&domain.Plugin{
		ID: "hello", Name: "Hello", Route: "/hello", Type: domain.PluginTypeNative, IsActive: false,
	}))
	// 仅改 permission（带空串显式清空也合法），不触发生命周期
	w := pluginDo(t, r, http.MethodPut, "/plugins/hello", map[string]any{"permission": "vm:view"})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, host.enabled)
	got, err := repo.FindByID("hello")
	require.NoError(t, err)
	assert.Equal(t, "vm:view", got.Permission)
}

func TestPlugin_Native_UpdateNoChangeSkipsHost(t *testing.T) {
	host := &stubNativeHost{}
	r, repo, _ := setupNativeRouter(t, host)
	require.NoError(t, repo.Save(&domain.Plugin{
		ID: "hello", Name: "Hello", Route: "/hello", Type: domain.PluginTypeNative, IsActive: true,
	}))
	w := pluginDo(t, r, http.MethodPut, "/plugins/hello", map[string]any{"is_active": true})
	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, host.enabled)
	assert.Empty(t, host.disabled, "启用状态未变化时不触发生命周期")
}

func TestPlugin_Native_UpdateHostError(t *testing.T) {
	host := &stubNativeHost{err: errors.New("spawn 失败")}
	r, repo, _ := setupNativeRouter(t, host)
	require.NoError(t, repo.Save(&domain.Plugin{
		ID: "hello", Name: "Hello", Route: "/hello", Type: domain.PluginTypeNative, IsActive: false,
	}))
	w := pluginDo(t, r, http.MethodPut, "/plugins/hello", map[string]any{"is_active": true})
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestPlugin_Native_DeleteRejected(t *testing.T) {
	host := &stubNativeHost{}
	r, repo, _ := setupNativeRouter(t, host)
	require.NoError(t, repo.Save(&domain.Plugin{
		ID: "hello", Name: "Hello", Route: "/hello", Type: domain.PluginTypeNative,
	}))
	w := pluginDo(t, r, http.MethodDelete, "/plugins/hello", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	_, err := repo.FindByID("hello")
	assert.NoError(t, err, "native 记录不可删除")
}

func TestPlugin_Restart_Native(t *testing.T) {
	host := &stubNativeHost{}
	r, repo, _ := setupNativeRouter(t, host)
	require.NoError(t, repo.Save(&domain.Plugin{
		ID: "hello", Name: "Hello", Route: "/hello", Type: domain.PluginTypeNative, IsActive: true,
	}))
	w := pluginDo(t, r, http.MethodPost, "/plugins/hello/restart", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{"hello"}, host.restarted)
}

func TestPlugin_Restart_NonNativeRejected(t *testing.T) {
	r, repo, _ := setupNativeRouter(t, &stubNativeHost{})
	require.NoError(t, repo.Save(&domain.Plugin{
		ID: "esxi", Name: "ESXi", Route: "/esxi-admin", Type: domain.PluginTypeAccess,
	}))
	w := pluginDo(t, r, http.MethodPost, "/plugins/esxi/restart", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPlugin_Restart_NoHost(t *testing.T) {
	r, repo, _ := setupNativeRouter(t, nil)
	require.NoError(t, repo.Save(&domain.Plugin{
		ID: "hello", Name: "Hello", Route: "/hello", Type: domain.PluginTypeNative,
	}))
	w := pluginDo(t, r, http.MethodPost, "/plugins/hello/restart", nil)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestPlugin_AccessValidation(t *testing.T) {
	r, _ := setupPlugin(t)
	cases := []struct {
		name string
		body map[string]any
		want int
	}{
		{"无 iframe 无 api → 400", map[string]any{"id": "a", "name": "x", "route": "/a", "type": "access"}, http.StatusBadRequest},
		{"iframe 相对路径 → 200", map[string]any{"id": "b", "name": "x", "route": "/b", "type": "access", "iframe_url": "/esxi/ui/"}, http.StatusOK},
		{"api_url 但无端点 → 400", map[string]any{"id": "c", "name": "x", "route": "/c", "type": "access", "api_url": "http://127.0.0.1:1"}, http.StatusBadRequest},
		{"api_url + 端点 → 200", map[string]any{"id": "d", "name": "x", "route": "/d", "type": "access", "api_url": "http://127.0.0.1:1", "endpoints": []any{map[string]any{"method": "GET", "path": "/api/info"}}}, http.StatusOK},
		{"iframe 与 api 共存 → 200", map[string]any{"id": "e", "name": "x", "route": "/e", "type": "access", "iframe_url": "https://ha.local", "api_url": "http://127.0.0.1:1", "endpoints": []any{map[string]any{"method": "GET", "path": "/api/info"}}}, http.StatusOK},
		{"非法 scheme → 400", map[string]any{"id": "f", "name": "x", "route": "/f", "type": "access", "iframe_url": "file:///etc/passwd"}, http.StatusBadRequest},
		{"协议相对 // 地址 → 400", map[string]any{"id": "g", "name": "x", "route": "/g", "type": "access", "iframe_url": "//evil.com/"}, http.StatusBadRequest},
		{"ID 含路径分隔符 → 400", map[string]any{"id": "../evil", "name": "x", "route": "/x", "type": "access", "iframe_url": "/esxi/ui/"}, http.StatusBadRequest},
		{"端点路径非 / 开头 → 400", map[string]any{"id": "h", "name": "x", "route": "/h", "type": "access", "api_url": "http://127.0.0.1:1", "endpoints": []any{map[string]any{"method": "GET", "path": "api/info"}}}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := pluginDo(t, r, http.MethodPost, "/plugins", tc.body)
			assert.Equal(t, tc.want, w.Code)
		})
	}
}
