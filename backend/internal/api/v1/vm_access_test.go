package v1

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/adapters/memory"
	"portalt/internal/adapters/mock"
	"portalt/internal/domain"
	"portalt/internal/domain/services"
)

// setupVMAuthEnv 构造带资源授权表的处理器级环境。
// fullUser 携带 vm:manage（放行全部）；limitedUser 仅含 vm:view，受 vm_access 约束。
func setupVMAuthEnv(t *testing.T, access *memory.VMAccessRepository) (*gin.Engine, *memory.VMAccessRepository) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	repo := memory.NewVMRepository()
	provider := mock.NewProvider(nil)
	svc := services.NewVMService(repo, provider)
	_, err := svc.SyncVMs(t.Context())
	require.NoError(t, err)
	handler := NewVMHandler(svc, access)

	admin := &domain.User{ID: "full", Username: "full", Role: domain.RoleAdmin}
	limited := &domain.User{ID: "limited", Username: "limited", Role: domain.RoleUser}
	fullPerms := map[string]struct{}{domain.PERM_VM_MANAGE: {}}
	viewPerms := map[string]struct{}{domain.PERM_VM_VIEW: {}}

	r := gin.New()
	// 按 Authorization 头区分用户上下文（full/limited）
	r.Use(func(c *gin.Context) {
		switch c.GetHeader("Authorization") {
		case "Bearer full":
			c.Set("auth.user", admin)
			c.Set("auth.perms", fullPerms)
		default:
			c.Set("auth.user", limited)
			c.Set("auth.perms", viewPerms)
		}
		c.Next()
	})
	r.GET("/vms", handler.List)
	r.GET("/vms/:id", handler.Get)
	r.GET("/vms/:id/status", handler.Status)
	r.POST("/vms/:id/start", handler.Start)
	r.PUT("/vms/:id/metadata", handler.UpdateMetadata)
	return r, access
}

func TestVMAuth_List_FiltersByAccess(t *testing.T) {
	access := memory.NewVMAccessRepository()
	require.NoError(t, access.SetForUser("limited", []string{"vm-mock-1"}))
	r, _ := setupVMAuthEnv(t, access)

	// 受限用户只看到授权的 VM
	req := httptest.NewRequest(http.MethodGet, "/vms", nil)
	req.Header.Set("Authorization", "Bearer limited")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	body := vmBody(t, w)
	data := body["data"].([]any)
	require.Len(t, data, 1)
	assert.Equal(t, "vm-mock-1", data[0].(map[string]any)["id"])

	// vm:manage 用户看到全部
	req = httptest.NewRequest(http.MethodGet, "/vms", nil)
	req.Header.Set("Authorization", "Bearer full")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	body = vmBody(t, w)
	assert.Len(t, body["data"].([]any), 3)
}

func TestVMAuth_Unauthorized_BehavesAsNotFound(t *testing.T) {
	access := memory.NewVMAccessRepository()
	require.NoError(t, access.SetForUser("limited", []string{"vm-mock-2"}))
	r, _ := setupVMAuthEnv(t, access)

	req := httptest.NewRequest(http.MethodGet, "/vms/vm-mock-1", nil)
	req.Header.Set("Authorization", "Bearer limited")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)

	// 电源操作同样 404（防枚举）
	req = httptest.NewRequest(http.MethodPost, "/vms/vm-mock-1/start", nil)
	req.Header.Set("Authorization", "Bearer limited")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)

	// 授权后的 VM 可正常获取
	req = httptest.NewRequest(http.MethodGet, "/vms/vm-mock-2", nil)
	req.Header.Set("Authorization", "Bearer limited")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestVMAuth_FullAccess_BypassesResourceCheck(t *testing.T) {
	access := memory.NewVMAccessRepository()
	r, _ := setupVMAuthEnv(t, access)

	req := httptest.NewRequest(http.MethodGet, "/vms/vm-mock-1", nil)
	req.Header.Set("Authorization", "Bearer full")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
}

func TestVMAccessHandler_Get_Set(t *testing.T) {
	gin.SetMode(gin.TestMode)
	access := memory.NewVMAccessRepository()
	h := NewVMAccessHandler(access)

	r := gin.New()
	r.GET("/users/:id/vm-access", h.Get)
	r.PUT("/users/:id/vm-access", h.Set)

	// 设置授权
	w := usersDo(t, r, http.MethodPut, "/users/u1/vm-access", map[string]any{"vm_ids": []string{"vm-a", "vm-b"}})
	require.Equal(t, http.StatusOK, w.Code)

	// 读取
	w = usersDo(t, r, http.MethodGet, "/users/u1/vm-access", nil)
	require.Equal(t, http.StatusOK, w.Code)
	body := vmBody(t, w)
	assert.ElementsMatch(t, []any{"vm-a", "vm-b"}, body["data"].(map[string]any)["vm_ids"].([]any))

	// 清空
	w = usersDo(t, r, http.MethodPut, "/users/u1/vm-access", map[string]any{"vm_ids": []string{}})
	require.Equal(t, http.StatusOK, w.Code)
	w = usersDo(t, r, http.MethodGet, "/users/u1/vm-access", nil)
	require.Equal(t, http.StatusOK, w.Code)
	body = vmBody(t, w)
	assert.Empty(t, body["data"].(map[string]any)["vm_ids"].([]any))
}
