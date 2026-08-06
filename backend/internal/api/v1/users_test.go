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
	"portalt/internal/api/middleware"
	"portalt/internal/domain"
	"portalt/internal/domain/services"
	"portalt/internal/ports"
)

func setupUsers(t *testing.T) (*gin.Engine, *memory.UserRepository, *memory.VMAccessRepository) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := memory.NewUserRepository()
	roleRepo := memory.NewRoleRepository()
	require.NoError(t, services.EnsureDefaultRoles(t.Context(), roleRepo))
	vmAccessRepo := memory.NewVMAccessRepository()
	h := NewUserHandler(repo, roleRepo, vmAccessRepo)
	admin := &domain.User{ID: "me", Username: "admin", Role: domain.RoleAdmin}
	require.NoError(t, repo.Save(admin))

	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("auth.user", admin); c.Next() })
	r.GET("/users", h.List)
	r.POST("/users", h.Create)
	r.PUT("/users/:id", h.Update)
	r.DELETE("/users/:id", h.Delete)
	return r, repo, vmAccessRepo
}

func TestUser_Create_List_Update_Delete(t *testing.T) {
	r, repo, _ := setupUsers(t)

	w := usersDo(t, r, http.MethodPost, "/users", map[string]any{
		"username": "alice", "password": "secret123", "email": "a@x.io", "role": "user",
	})
	require.Equal(t, http.StatusOK, w.Code)
	var created map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	uid := created["data"].(map[string]any)["id"].(string)
	assert.Equal(t, "alice", created["data"].(map[string]any)["username"])

	// 密码必须 bcrypt 存储且可登录
	u, err := repo.FindByUsername("alice")
	require.NoError(t, err)
	assert.NotEqual(t, "secret123", u.Password)
	assert.Equal(t, domain.RoleUser, u.Role)

	// 重名冲突
	w = usersDo(t, r, http.MethodPost, "/users", map[string]any{
		"username": "alice", "password": "x12345678",
	})
	assert.Equal(t, http.StatusConflict, w.Code)

	// 列表
	w = usersDo(t, r, http.MethodGet, "/users", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var list map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	assert.Len(t, list["data"].([]any), 2)

	// 更新角色 + 重置密码
	w = usersDo(t, r, http.MethodPut, "/users/"+uid, map[string]any{
		"role": "viewer", "password": "newpass456",
	})
	require.Equal(t, http.StatusOK, w.Code)
	updated, err := repo.FindByID(uid)
	require.NoError(t, err)
	assert.Equal(t, domain.RoleViewer, updated.Role)
	assert.NotEqual(t, "newpass456", updated.Password)

	// 非法角色
	w = usersDo(t, r, http.MethodPut, "/users/"+uid, map[string]any{"role": "root"})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 不能删除自己
	w = usersDo(t, r, http.MethodDelete, "/users/me", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 删除
	w = usersDo(t, r, http.MethodDelete, "/users/"+uid, nil)
	require.Equal(t, http.StatusOK, w.Code)
	_, err = repo.FindByID(uid)
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

// 删除用户时同步清理其资源授权（vm_access），防止 ID 复用后授权复活。
func TestUser_Delete_CleansVMAccess(t *testing.T) {
	r, _, vmAccessRepo := setupUsers(t)

	w := usersDo(t, r, http.MethodPost, "/users", map[string]any{
		"username": "alice", "password": "secret123",
	})
	require.Equal(t, http.StatusOK, w.Code)
	var created map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	uid := created["data"].(map[string]any)["id"].(string)

	require.NoError(t, vmAccessRepo.SetForUser(uid, []string{"vm-1", "vm-2"}))
	set, err := vmAccessRepo.VisibleVMIDs(uid)
	require.NoError(t, err)
	assert.Len(t, set, 2)

	w = usersDo(t, r, http.MethodDelete, "/users/"+uid, nil)
	require.Equal(t, http.StatusOK, w.Code)

	set, err = vmAccessRepo.VisibleVMIDs(uid)
	require.NoError(t, err)
	assert.Empty(t, set)
}

func TestUser_Create_MissingFields(t *testing.T) {
	r, _, _ := setupUsers(t)
	w := usersDo(t, r, http.MethodPost, "/users", map[string]any{"username": "bob"})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestUser_Update_NotFound(t *testing.T) {
	r, _, _ := setupUsers(t)
	w := usersDo(t, r, http.MethodPut, "/users/nope", map[string]any{"email": "x@y.io"})
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func setupRoles(t *testing.T) (*gin.Engine, *middleware.RoleLoader, *memory.RoleRepository) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	repo := memory.NewRoleRepository()
	require.NoError(t, services.EnsureDefaultRoles(t.Context(), repo))
	loader := middleware.NewRoleLoader(repo)
	perms := memory.NewPermissionRepository()
	require.NoError(t, perms.EnsureDefault(domain.AllPermissions()))
	h := NewRoleHandler(repo, perms, loader)

	r := gin.New()
	r.GET("/roles", h.List)
	r.GET("/roles/permissions", h.Permissions)
	r.POST("/roles", h.Create)
	r.PUT("/roles/:id", h.Update)
	r.DELETE("/roles/:id", h.Delete)
	return r, loader, repo
}

func TestRole_List_And_Matrix(t *testing.T) {
	r, loader, repo := setupRoles(t)

	w := usersDo(t, r, http.MethodGet, "/roles", nil)
	require.Equal(t, http.StatusOK, w.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	roles := body["data"].([]any)
	require.Len(t, roles, 3)

	// 权限字典
	w = usersDo(t, r, http.MethodGet, "/roles/permissions", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	perms := body["data"].([]any)
	assert.NotEmpty(t, perms)

	// 修改 viewer 权限矩阵
	w = usersDo(t, r, http.MethodPut, "/roles/viewer", map[string]any{
		"name": "访客", "description": "只读",
		"permissions": []string{"vm:view", "plugin:view"},
	})
	require.Equal(t, http.StatusOK, w.Code)

	// 缓存已失效，加载器读到新矩阵
	assert.Equal(t, []string{"vm:view", "plugin:view"}, loader.PermissionsFor(domain.RoleViewer))
	got, err := repo.FindByID("viewer")
	require.NoError(t, err)
	assert.Equal(t, "只读", got.Description)

	// 去重 + 空值过滤
	w = usersDo(t, r, http.MethodPut, "/roles/viewer", map[string]any{
		"name": "访客", "permissions": []string{"vm:view", "vm:view", ""},
	})
	require.Equal(t, http.StatusOK, w.Code)
	got, err = repo.FindByID("viewer")
	require.NoError(t, err)
	assert.Equal(t, []string{"vm:view"}, got.Permissions)
}

func TestRole_Delete_BuiltinForbidden(t *testing.T) {
	r, _, _ := setupRoles(t)
	w := usersDo(t, r, http.MethodDelete, "/roles/admin", nil)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 自定义角色可删除
	repo2 := memory.NewRoleRepository()
	require.NoError(t, repo2.Save(&domain.RoleDefinition{ID: "ops", Name: "运维", Permissions: []string{"vm:view"}}))
	loader2 := middleware.NewRoleLoader(repo2)
	h := NewRoleHandler(repo2, memory.NewPermissionRepository(), loader2)
	r2 := gin.New()
	r2.DELETE("/roles/:id", h.Delete)
	w = usersDo(t, r2, http.MethodDelete, "/roles/ops", nil)
	require.Equal(t, http.StatusOK, w.Code)
	_, err := repo2.FindByID("ops")
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

func TestRole_Update_NotFound(t *testing.T) {
	r, _, _ := setupRoles(t)
	w := usersDo(t, r, http.MethodPut, "/roles/nope", map[string]any{"name": "x"})
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRole_Create(t *testing.T) {
	r, loader, repo := setupRoles(t)

	// 成功创建（权限来自字典，去重 + 保序）
	w := usersDo(t, r, http.MethodPost, "/roles", map[string]any{
		"id": "ops", "name": "运维", "permissions": []string{"vm:console", "vm:console", "vm:view"},
	})
	require.Equal(t, http.StatusOK, w.Code)
	got, err := repo.FindByID("ops")
	require.NoError(t, err)
	assert.Equal(t, []string{"vm:console", "vm:view"}, got.Permissions)
	assert.Equal(t, "运维", got.Name)

	// 重复创建冲突
	w = usersDo(t, r, http.MethodPost, "/roles", map[string]any{"id": "ops", "name": "运维2"})
	assert.Equal(t, http.StatusConflict, w.Code)

	// 非法 ID 格式
	w = usersDo(t, r, http.MethodPost, "/roles", map[string]any{"id": "Bad Role!", "name": "x"})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 内置角色不可重复创建
	w = usersDo(t, r, http.MethodPost, "/roles", map[string]any{"id": "admin", "name": "x"})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 权限不在字典 → 400
	w = usersDo(t, r, http.MethodPost, "/roles", map[string]any{
		"id": "evil", "name": "x", "permissions": []string{"vm:destroy"},
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// 创建的角色可被用户引用（loader 校验）
	loader.Invalidate("ops")
	w = usersDo(t, r, http.MethodPut, "/roles/ops", map[string]any{
		"name": "运维", "permissions": []string{"vm:view"},
	})
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestUser_Create_With_CustomRole(t *testing.T) {
	_, repo, _ := setupUsers(t)

	// 角色表已存在自定义角色
	roleRepo := memory.NewRoleRepository()
	require.NoError(t, services.EnsureDefaultRoles(t.Context(), roleRepo))
	require.NoError(t, roleRepo.Save(&domain.RoleDefinition{ID: "ops", Name: "运维", Permissions: []string{"vm:view"}}))

	h := NewUserHandler(repo, roleRepo, memory.NewVMAccessRepository())
	r2 := gin.New()
	r2.Use(func(c *gin.Context) { c.Set("auth.user", &domain.User{ID: "me", Username: "admin", Role: domain.RoleAdmin}); c.Next() })
	r2.POST("/users", h.Create)

	w := usersDo(t, r2, http.MethodPost, "/users", map[string]any{
		"username": "ops1", "password": "secret123", "role": "ops",
	})
	require.Equal(t, http.StatusOK, w.Code)
	u, err := repo.FindByUsername("ops1")
	require.NoError(t, err)
	assert.Equal(t, domain.Role("ops"), u.Role)

	// 不存在的角色 → 400
	w = usersDo(t, r2, http.MethodPost, "/users", map[string]any{
		"username": "ops2", "password": "secret123", "role": "ghost",
	})
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// 复用插件测试的请求辅助函数
func usersDo(t *testing.T, r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
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
