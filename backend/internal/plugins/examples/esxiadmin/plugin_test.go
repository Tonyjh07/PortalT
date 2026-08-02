package esxiadmin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/domain"
	"portalt/internal/plugins"
)

var errNotFound = errors.New("not found")

type fakeVMService struct {
	vms  []*domain.VM
	host *domain.HostInfo
}

func (f *fakeVMService) GetVMStatus(_ context.Context, _ string) (*domain.VM, error) { return nil, nil }
func (f *fakeVMService) ListVMs(_ context.Context) ([]*domain.VM, error)             { return f.vms, nil }
func (f *fakeVMService) GetHostInfo(_ context.Context) (*domain.HostInfo, error)     { return f.host, nil }

func (f *fakeVMService) StartVM(_ context.Context, id string) (*domain.VM, error) {
	for _, vm := range f.vms {
		if vm.ID == id {
			if vm.Status != domain.VMStatusPoweredOff && vm.Status != domain.VMStatusSuspended {
				return nil, errInvalidOp
			}
			vm.Status = domain.VMStatusPoweredOn
			return vm, nil
		}
	}
	return nil, errNotFound
}

func (f *fakeVMService) StopVM(_ context.Context, id string) (*domain.VM, error) {
	for _, vm := range f.vms {
		if vm.ID == id {
			vm.Status = domain.VMStatusPoweredOff
			return vm, nil
		}
	}
	return nil, errNotFound
}

func (f *fakeVMService) RestartVM(_ context.Context, id string) (*domain.VM, error) {
	return f.StartVM(context.Background(), id)
}

var errInvalidOp = errors.New("invalid operation")

func setupRouter(vm *fakeVMService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("auth.user", &domain.User{ID: "u1", Username: "admin", Role: domain.RoleAdmin})
		c.Next()
	})
	p := New()
	p.Mount(r.Group("/plugins/native/esxi-admin"), plugins.Deps{VMs: vm})
	return r
}

func TestEsxiAdmin_Host(t *testing.T) {
	r := setupRouter(&fakeVMService{host: &domain.HostInfo{Name: "esxi-1", Version: "8.0"}})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/plugins/native/esxi-admin/host", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Data domain.HostInfo `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "esxi-1", body.Data.Name)
	assert.Equal(t, "8.0", body.Data.Version)
}

func TestEsxiAdmin_ListAndPower(t *testing.T) {
	vmSvc := &fakeVMService{vms: []*domain.VM{
		{ID: "vm1", Name: "app-1", Status: domain.VMStatusPoweredOn, CPU: 2, MemoryMB: 4096},
		{ID: "vm2", Name: "db-1", Status: domain.VMStatusPoweredOff},
	}}
	r := setupRouter(vmSvc)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/plugins/native/esxi-admin/vms", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "app-1")

	// 启动 vm2（当前已关机）
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/plugins/native/esxi-admin/vms/vm2/start", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "poweredOn")

	// 对已运行 VM 执行 start → 4007（状态规则拦截）
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/plugins/native/esxi-admin/vms/vm1/start", nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "4007")

	// 不存在 VM → 400
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/plugins/native/esxi-admin/vms/nope/stop", nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
