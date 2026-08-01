package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authadapter "portalt/internal/adapters/auth"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

// seedPlugins 写入示例插件（admin 可全见，viewer 只见无权限要求项）。
func seedPlugins(t *testing.T, env *testEnv) {
	t.Helper()
	require.NoError(t, env.plugins.Save(&domain.Plugin{
		ID: "p-1", Name: "仪表盘", Route: "/", SortOrder: 1, IsActive: true,
	}))
	require.NoError(t, env.plugins.Save(&domain.Plugin{
		ID: "p-2", Name: "用户管理", Route: "/users", SortOrder: 2, IsActive: true,
		Permission: domain.PERM_USER_MANAGE,
	}))
	require.NoError(t, env.plugins.Save(&domain.Plugin{
		ID: "p-3", Name: "停用", Route: "/off", SortOrder: 3, IsActive: false,
	}))
}

// createUser 创建普通用户（含 vm:start/plugin:view，无 user:manage 等管理权限）。
func createUser(t *testing.T, env *testEnv) string {
	t.Helper()
	hash, err := authadapter.HashPassword("user123")
	require.NoError(t, err)
	user := &domain.User{
		ID:       "u-user",
		Username: "bob",
		Password: hash,
		Role:     domain.RoleUser,
	}
	require.NoError(t, env.userRepo.Save(user))
	return loginToken(t, env, "bob", "user123")
}

// createViewer 创建查看者用户（仅 vm:view，无操作权限）。
func createViewer(t *testing.T, env *testEnv) string {
	t.Helper()
	hash, err := authadapter.HashPassword("viewer123")
	require.NoError(t, err)
	user := &domain.User{
		ID:       "u-viewer",
		Username: "viewer",
		Password: hash,
		Role:     domain.RoleViewer,
	}
	require.NoError(t, env.userRepo.Save(user))
	return loginToken(t, env, "viewer", "viewer123")
}

// loginToken 登录并返回访问令牌。
func loginToken(t *testing.T, env *testEnv, username, password string) string {
	t.Helper()
	code, body := login(t, env.router, username, password)
	require.Equal(t, http.StatusOK, code)
	return body["data"].(map[string]any)["access_token"].(string)
}

func TestVMs_List_RequiresAuth(t *testing.T) {
	env := setupTestEnv(t)
	w := doRequest(t, env.router, http.MethodGet, "/api/v1/vms", nil, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, float64(4003), unmarshalCode(t, w))
}

func TestVMs_List_Admin(t *testing.T) {
	env := setupTestEnv(t)
	require.NoError(t, env.vmRepo.Save(&domain.VM{ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOn}))

	token := loginAndToken(t, env)
	w := doRequest(t, env.router, http.MethodGet, "/api/v1/vms", nil, token)
	require.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Data []domain.VM `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	assert.Equal(t, "vm-1", body.Data[0].ID)
}

func TestVMs_Start_ViewerForbidden(t *testing.T) {
	env := setupTestEnv(t)
	require.NoError(t, env.vmRepo.Save(&domain.VM{ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOff}))

	token := createViewer(t, env)
	w := doRequest(t, env.router, http.MethodPost, "/api/v1/vms/vm-1/start", nil, token)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Equal(t, float64(4005), unmarshalCode(t, w))
}

func TestVMs_Start_UserRoleAllowed(t *testing.T) {
	env := setupTestEnv(t)
	require.NoError(t, env.vmRepo.Save(&domain.VM{ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOff}))
	env.provider.SetVMs([]*domain.VM{{ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOff}})

	token := createUser(t, env)
	w := doRequest(t, env.router, http.MethodPost, "/api/v1/vms/vm-1/start", nil, token)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestVMs_Start_Admin_EndToEnd(t *testing.T) {
	env := setupTestEnv(t)
	require.NoError(t, env.vmRepo.Save(&domain.VM{ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOff}))
	env.provider.SetVMs([]*domain.VM{{ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOff}})

	token := loginAndToken(t, env)
	w := doRequest(t, env.router, http.MethodPost, "/api/v1/vms/vm-1/start", nil, token)
	require.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Data domain.VM `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, domain.VMStatusPoweredOn, body.Data.Status)

	// 状态端点反映变更
	w = doRequest(t, env.router, http.MethodGet, "/api/v1/vms/vm-1/status", nil, token)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, domain.VMStatusPoweredOn, body.Data.Status)
}

func TestVMs_Stop_InvalidState_Conflict(t *testing.T) {
	env := setupTestEnv(t)
	require.NoError(t, env.vmRepo.Save(&domain.VM{ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOff}))
	env.provider.SetVMs([]*domain.VM{{ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOff}})

	token := loginAndToken(t, env)
	w := doRequest(t, env.router, http.MethodPost, "/api/v1/vms/vm-1/stop", nil, token)
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, float64(4007), unmarshalCode(t, w))
}

func TestVMs_NotFound(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAndToken(t, env)

	w := doRequest(t, env.router, http.MethodGet, "/api/v1/vms/ghost", nil, token)
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, float64(4006), unmarshalCode(t, w))
}

func TestMenu_Admin_FullTree(t *testing.T) {
	env := setupTestEnv(t)
	seedPlugins(t, env)

	token := loginAndToken(t, env)
	w := doRequest(t, env.router, http.MethodGet, "/api/v1/menu", nil, token)
	require.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Data []domain.Plugin `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data, 2) // 仅已启用
	assert.Equal(t, "p-1", body.Data[0].ID)
	assert.Equal(t, "p-2", body.Data[1].ID)
}

func TestMenu_UserRole_Filtered(t *testing.T) {
	env := setupTestEnv(t)
	seedPlugins(t, env)

	token := createUser(t, env)
	w := doRequest(t, env.router, http.MethodGet, "/api/v1/menu", nil, token)
	require.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Data []domain.Plugin `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	assert.Equal(t, "p-1", body.Data[0].ID)
}

func TestMenu_ViewerForbidden(t *testing.T) {
	env := setupTestEnv(t)
	seedPlugins(t, env)

	token := createViewer(t, env)
	w := doRequest(t, env.router, http.MethodGet, "/api/v1/menu", nil, token)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestPlugin_CRUD_Admin(t *testing.T) {
	env := setupTestEnv(t)
	token := loginAndToken(t, env)

	// 创建
	w := doRequest(t, env.router, http.MethodPost, "/api/v1/plugins", map[string]any{
		"name": "Proxmox", "route": "/pve", "sort_order": 5, "is_active": true,
	}, token)
	require.Equal(t, http.StatusOK, w.Code)
	var created struct {
		Data domain.Plugin `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.NotEmpty(t, created.Data.ID)

	// 更新
	w = doRequest(t, env.router, http.MethodPut, "/api/v1/plugins/"+created.Data.ID, map[string]any{
		"name": "Proxmox VE", "route": "/pve", "is_active": false,
	}, token)
	require.Equal(t, http.StatusOK, w.Code)
	var updated struct {
		Data domain.Plugin `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	assert.Equal(t, "Proxmox VE", updated.Data.Name)
	assert.False(t, updated.Data.IsActive)

	// 删除
	w = doRequest(t, env.router, http.MethodDelete, "/api/v1/plugins/"+created.Data.ID, nil, token)
	require.Equal(t, http.StatusOK, w.Code)
	_, err := env.plugins.FindByID(created.Data.ID)
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

func TestPlugin_Manage_ViewerForbidden(t *testing.T) {
	env := setupTestEnv(t)
	token := createViewer(t, env)

	w := doRequest(t, env.router, http.MethodPost, "/api/v1/plugins", map[string]any{
		"name": "x", "route": "/x",
	}, token)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestPlugins_List_ViewerForbidden(t *testing.T) {
	env := setupTestEnv(t)
	token := createViewer(t, env)

	w := doRequest(t, env.router, http.MethodGet, "/api/v1/plugins", nil, token)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestAuth_NewUserCanLogin(t *testing.T) {
	env := setupTestEnv(t)
	// 新用户（非管理员）登录与令牌链路
	hash, err := authadapter.HashPassword("alice123")
	require.NoError(t, err)
	user := &domain.User{
		ID: "u-user", Username: "alice", Password: hash, Role: domain.RoleUser,
	}
	require.NoError(t, env.userRepo.Save(user))

	code, body := login(t, env.router, "alice", "alice123")
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, "user", body["data"].(map[string]any)["user"].(map[string]any)["role"])

	// 密码错误仍拒绝
	code, _ = login(t, env.router, "alice", "wrong")
	assert.Equal(t, http.StatusUnauthorized, code)
}
