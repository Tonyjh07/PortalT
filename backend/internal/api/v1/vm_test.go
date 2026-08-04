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
	"portalt/internal/adapters/mock"
	"portalt/internal/domain"
	"portalt/internal/domain/services"
)

// vmEnv 处理器级测试环境：内存仓储 + mock 提供者。
type vmEnv struct {
	router   *gin.Engine
	repo     *memory.VMRepository
	provider *mock.Provider
}

func setupVMEnv(t *testing.T) *vmEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	repo := memory.NewVMRepository()
	provider := mock.NewProvider(nil)
	svc := services.NewVMService(repo, provider)
	// 将提供者示例 VM 同步进仓储
	_, err := svc.SyncVMs(t.Context())
	require.NoError(t, err)
	handler := NewVMHandler(svc)

	r := gin.New()
	r.GET("/vms", handler.List)
	r.GET("/vms/:id", handler.Get)
	r.GET("/vms/:id/status", handler.Status)
	r.POST("/vms/:id/start", handler.Start)
	r.POST("/vms/:id/stop", handler.Stop)
	r.POST("/vms/:id/restart", handler.Restart)
	r.PUT("/vms/:id/metadata", handler.UpdateMetadata)

	return &vmEnv{router: r, repo: repo, provider: provider}
}

func vmRequest(router *gin.Engine, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer test")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func vmBody(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body
}

func TestVMHandler_List(t *testing.T) {
	env := setupVMEnv(t)
	w := vmRequest(env.router, http.MethodGet, "/vms")
	require.Equal(t, http.StatusOK, w.Code)

	body := vmBody(t, w)
	data := body["data"].([]any)
	require.Len(t, data, 3)
}

func TestVMHandler_Get(t *testing.T) {
	env := setupVMEnv(t)
	w := vmRequest(env.router, http.MethodGet, "/vms/vm-mock-1")
	require.Equal(t, http.StatusOK, w.Code)

	vm := vmBody(t, w)["data"].(map[string]any)
	assert.Equal(t, "mock-vm-1", vm["name"])
	assert.Equal(t, "poweredOn", vm["status"])
}

func TestVMHandler_Get_NotFound(t *testing.T) {
	env := setupVMEnv(t)
	w := vmRequest(env.router, http.MethodGet, "/vms/ghost")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Equal(t, float64(4006), vmBody(t, w)["code"])
}

func TestVMHandler_Status(t *testing.T) {
	env := setupVMEnv(t)
	w := vmRequest(env.router, http.MethodGet, "/vms/vm-mock-1/status")
	require.Equal(t, http.StatusOK, w.Code)

	st := vmBody(t, w)["data"].(map[string]any)
	assert.Equal(t, "vm-mock-1", st["id"])
	assert.Equal(t, "poweredOn", st["status"])
}

func TestVMHandler_Stop_Success(t *testing.T) {
	env := setupVMEnv(t)
	w := vmRequest(env.router, http.MethodPost, "/vms/vm-mock-1/stop")
	require.Equal(t, http.StatusOK, w.Code)

	vm := vmBody(t, w)["data"].(map[string]any)
	assert.Equal(t, "poweredOff", vm["status"])

	// 状态已持久化
	stored, err := env.repo.FindByID("vm-mock-1")
	require.NoError(t, err)
	assert.Equal(t, domain.VMStatusPoweredOff, stored.Status)
}

func TestVMHandler_Start_FromPoweredOff(t *testing.T) {
	env := setupVMEnv(t)
	// 先停止再启动
	require.Equal(t, http.StatusOK, vmRequest(env.router, http.MethodPost, "/vms/vm-mock-1/stop").Code)

	w := vmRequest(env.router, http.MethodPost, "/vms/vm-mock-1/start")
	require.Equal(t, http.StatusOK, w.Code)
	vm := vmBody(t, w)["data"].(map[string]any)
	assert.Equal(t, "poweredOn", vm["status"])
}

func TestVMHandler_Start_AlreadyRunning_Conflict(t *testing.T) {
	env := setupVMEnv(t)
	w := vmRequest(env.router, http.MethodPost, "/vms/vm-mock-1/start")
	assert.Equal(t, http.StatusConflict, w.Code)
	assert.Equal(t, float64(4007), vmBody(t, w)["code"])
}

func TestVMHandler_Stop_NotRunning_Conflict(t *testing.T) {
	env := setupVMEnv(t)
	require.Equal(t, http.StatusOK, vmRequest(env.router, http.MethodPost, "/vms/vm-mock-1/stop").Code)

	w := vmRequest(env.router, http.MethodPost, "/vms/vm-mock-1/stop")
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestVMHandler_Restart_Success(t *testing.T) {
	env := setupVMEnv(t)
	w := vmRequest(env.router, http.MethodPost, "/vms/vm-mock-1/restart")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "poweredOn", vmBody(t, w)["data"].(map[string]any)["status"])
}

func TestVMHandler_PowerOp_NotFound(t *testing.T) {
	env := setupVMEnv(t)
	w := vmRequest(env.router, http.MethodPost, "/vms/ghost/start")
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestVMHandler_UpdateMetadata_Success(t *testing.T) {
	env := setupVMEnv(t)
	body := strings.NewReader(`{"guac.protocol":"rdp","guac.hostname":"10.0.0.9","guac.port":null}`)
	req := httptest.NewRequest(http.MethodPut, "/vms/vm-mock-1/metadata", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	data := vmBody(t, w)["data"].(map[string]any)
	md := data["metadata"].(map[string]any)
	assert.Equal(t, "rdp", md["guac.protocol"])
	assert.Equal(t, "10.0.0.9", md["guac.hostname"])
	_, ok := md["guac.port"]
	assert.False(t, ok) // null 删除

	// 仓储持久化
	vm, err := env.repo.FindByID("vm-mock-1")
	require.NoError(t, err)
	assert.Equal(t, "rdp", vm.Metadata["guac.protocol"])
}

func TestVMHandler_UpdateMetadata_BadBody(t *testing.T) {
	env := setupVMEnv(t)
	req := httptest.NewRequest(http.MethodPut, "/vms/vm-mock-1/metadata", strings.NewReader(`not-json`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestVMHandler_UpdateMetadata_NotFound(t *testing.T) {
	env := setupVMEnv(t)
	req := httptest.NewRequest(http.MethodPut, "/vms/ghost/metadata", strings.NewReader(`{"a":"b"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestVMHandler_UpdateMetadata_InvalidProtocol(t *testing.T) {
	env := setupVMEnv(t)
	req := httptest.NewRequest(http.MethodPut, "/vms/vm-mock-1/metadata",
		strings.NewReader(`{"guac.protocol":"evil"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestVMHandler_UpdateMetadata_InvalidPort(t *testing.T) {
	env := setupVMEnv(t)
	req := httptest.NewRequest(http.MethodPut, "/vms/vm-mock-1/metadata",
		strings.NewReader(`{"guac.port":70000}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestVMHandler_UpdateMetadata_EmptyRustdeskID(t *testing.T) {
	env := setupVMEnv(t)
	req := httptest.NewRequest(http.MethodPut, "/vms/vm-mock-1/metadata",
		strings.NewReader(`{"rustdesk.id":"  "}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestVMHandler_MetadataSanitized(t *testing.T) {
	env := setupVMEnv(t)
	// 保存含密码的 metadata
	req := httptest.NewRequest(http.MethodPut, "/vms/vm-mock-1/metadata",
		strings.NewReader(`{"guac.protocol":"rdp","guac.password":"s3cret","guac.hostname":"10.0.0.9"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	env.router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	md := vmBody(t, w)["data"].(map[string]any)["metadata"].(map[string]any)
	assert.NotContains(t, md, "guac.password") // 响应脱敏，密码只写不回
	assert.Equal(t, "rdp", md["guac.protocol"])

	// 仓储中仍保留完整数据（远程桌面连接仍需密码）
	stored, err := env.repo.FindByID("vm-mock-1")
	require.NoError(t, err)
	assert.Equal(t, "s3cret", stored.Metadata["guac.password"])

	// 详情接口同样脱敏
	w = vmRequest(env.router, http.MethodGet, "/vms/vm-mock-1")
	require.Equal(t, http.StatusOK, w.Code)
	md = vmBody(t, w)["data"].(map[string]any)["metadata"].(map[string]any)
	assert.NotContains(t, md, "guac.password")

	// 列表接口同样脱敏
	w = vmRequest(env.router, http.MethodGet, "/vms")
	require.Equal(t, http.StatusOK, w.Code)
	for _, item := range vmBody(t, w)["data"].([]any) {
		if vm := item.(map[string]any); vm["id"] == "vm-mock-1" {
			assert.NotContains(t, vm["metadata"].(map[string]any), "guac.password")
		}
	}
}
