package v1

import (
	"bytes"
	"encoding/json"
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
	handler := NewPluginHandler(repo, nil)

	r := gin.New()
	r.POST("/plugins", handler.Create)
	r.PUT("/plugins/:id", handler.Update)
	r.DELETE("/plugins/:id", handler.Delete)
	r.GET("/plugins", handler.List)
	return r, repo
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
	handler := NewPluginHandler(repo, perms)

	r := gin.New()
	r.POST("/plugins", handler.Create)

	// 字典内权限 → 200
	w := pluginDo(t, r, http.MethodPost, "/plugins", map[string]any{
		"name": "受限工具", "route": "/tool", "type": "proxy",
		"api_url": "http://127.0.0.1:1", "permission": "vm:view",
		"endpoints": []any{map[string]any{"method": "GET", "path": "/api/info"}},
	})
	assert.Equal(t, http.StatusOK, w.Code)

	// 字典外权限 → 400
	w = pluginDo(t, r, http.MethodPost, "/plugins", map[string]any{
		"name": "越权工具", "route": "/tool2", "type": "proxy",
		"api_url": "http://127.0.0.1:1", "permission": "vm:destroy",
		"endpoints": []any{map[string]any{"method": "GET", "path": "/api/info"}},
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPlugin_Update_Success(t *testing.T) {
	r, repo := setupPlugin(t)
	require.NoError(t, repo.Save(&domain.Plugin{ID: "p-1", Name: "old", Route: "/ha"}))

	w := pluginDo(t, r, http.MethodPut, "/plugins/p-1", map[string]any{
		"name": "new", "route": "/ha", "permission": "vm:view", "is_active": false,
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
