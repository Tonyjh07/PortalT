package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/adapters/memory"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

// stubProvider 测试用虚拟化提供者桩
type stubProvider struct {
	vms   []*domain.VM
	host  *domain.HostInfo
	err   error
	calls int
}

func (s *stubProvider) ListVMs() ([]*domain.VM, error) {
	s.calls++
	return s.vms, s.err
}

func (s *stubProvider) StartVM(id string) error {
	return s.applyOp(id, domain.VMStatusPoweredOn)
}
func (s *stubProvider) StopVM(id string) error {
	return s.applyOp(id, domain.VMStatusPoweredOff)
}
func (s *stubProvider) RestartVM(id string) error {
	return s.applyOp(id, domain.VMStatusPoweredOn)
}

// applyOp 将电源操作结果应用到桩提供者的 VM 集合（模拟真实平台行为）。
func (s *stubProvider) applyOp(id string, status domain.VMStatus) error {
	for _, vm := range s.vms {
		if vm.ID == id {
			vm.Status = status
			return nil
		}
	}
	return errors.New("provider: vm not found")
}

func (s *stubProvider) GetHostInfo() (*domain.HostInfo, error) {
	return s.host, s.err
}

// stubVMRepo 记录调用的内存仓储（注入错误用）
type stubVMRepo struct {
	ports.VMRepository
	saveErr    error
	findAllErr error
	deleteErr  error
}

func (r *stubVMRepo) Save(vm *domain.VM) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	return r.VMRepository.Save(vm)
}

func (r *stubVMRepo) FindAll() ([]*domain.VM, error) {
	if r.findAllErr != nil {
		return nil, r.findAllErr
	}
	return r.VMRepository.FindAll()
}

func (r *stubVMRepo) Delete(id string) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	return r.VMRepository.Delete(id)
}

func newTestService() (*VMService, *memory.VMRepository, *stubProvider) {
	repo := memory.NewVMRepository()
	provider := &stubProvider{}
	return NewVMService(repo, provider), repo, provider
}

func TestVMService_SyncVMs_SavesAll(t *testing.T) {
	svc, repo, provider := newTestService()
	provider.vms = []*domain.VM{
		{ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOn},
		{ID: "vm-2", Name: "db", Status: domain.VMStatusPoweredOff},
	}

	n, err := svc.SyncVMs(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 2, n)

	all, err := repo.FindAll()
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestVMService_SyncVMs_DeletesStale(t *testing.T) {
	svc, repo, provider := newTestService()
	require.NoError(t, repo.Save(&domain.VM{ID: "vm-old", Name: "stale"}))
	provider.vms = []*domain.VM{{ID: "vm-1", Name: "web"}}

	_, err := svc.SyncVMs(context.Background())
	require.NoError(t, err)

	_, err = repo.FindByID("vm-old")
	assert.ErrorIs(t, err, ports.ErrNotFound)
	all, err := repo.FindAll()
	require.NoError(t, err)
	assert.Len(t, all, 1)
}

func TestVMService_SyncVMs_ProviderError(t *testing.T) {
	svc, repo, _ := newTestService()
	// 提供者报错时，仓储不应有任何变更
	provider := &stubProvider{err: errors.New("esxi unreachable")}
	svc.provider = provider
	require.NoError(t, repo.Save(&domain.VM{ID: "vm-1", Name: "keep"}))

	n, err := svc.SyncVMs(context.Background())
	assert.Error(t, err)
	assert.Equal(t, 0, n)

	all, err := repo.FindAll()
	require.NoError(t, err)
	assert.Len(t, all, 1)
}

func TestVMService_SyncVMs_RepoError(t *testing.T) {
	// 仓储保存失败应返回错误
	svc, repo, provider := newTestService()
	provider.vms = []*domain.VM{{ID: "vm-1", Name: "web"}}
	svc.repo = &stubVMRepo{VMRepository: repo, saveErr: errors.New("db down")}

	_, err := svc.SyncVMs(context.Background())
	assert.Error(t, err)
}

func TestVMService_SyncVMs_DeleteError(t *testing.T) {
	svc, repo, provider := newTestService()
	provider.vms = []*domain.VM{{ID: "vm-1", Name: "web"}}
	require.NoError(t, repo.Save(&domain.VM{ID: "vm-old", Name: "stale"}))
	svc.repo = &stubVMRepo{VMRepository: repo, deleteErr: errors.New("db down")}

	_, err := svc.SyncVMs(context.Background())
	assert.Error(t, err)
}

func TestVMService_SyncVMs_EmptyProvider(t *testing.T) {
	// 提供者返回空列表 → 清空仓储
	svc, repo, provider := newTestService()
	provider.vms = []*domain.VM{}
	require.NoError(t, repo.Save(&domain.VM{ID: "vm-1", Name: "stale"}))

	n, err := svc.SyncVMs(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	all, err := repo.FindAll()
	require.NoError(t, err)
	assert.Empty(t, all)
}

func TestVMService_SyncVMs_SkipsInvalid(t *testing.T) {
	svc, repo, provider := newTestService()
	provider.vms = []*domain.VM{
		{ID: "vm-1", Name: "ok"},
		{Name: "no-id"}, // 空ID应跳过
		nil,
	}

	n, err := svc.SyncVMs(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 3, n) // 返回原始数量

	all, err := repo.FindAll()
	require.NoError(t, err)
	assert.Len(t, all, 1)
}

func TestVMService_SyncVMs_MergesMetadata(t *testing.T) {
	svc, repo, provider := newTestService()
	// 库内已有手动配置的远程桌面参数
	require.NoError(t, repo.Save(&domain.VM{
		ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOn,
		Metadata: map[string]any{
			"guac.protocol": "vnc",
			"guac.hostname": "10.0.0.5",
			"guac.port":     "5900",
			"moid":          "old", // 模拟平台同步过的键
		},
	}))
	// 提供者返回的 VM 不含 guac.*，但带平台自己的键
	provider.vms = []*domain.VM{{
		ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOff,
		Metadata: map[string]any{"moid": "new"},
	}}

	_, err := svc.SyncVMs(context.Background())
	require.NoError(t, err)

	stored, err := repo.FindByID("vm-1")
	require.NoError(t, err)
	// 手动配置的 guac.* 保留，平台键以提供者为准
	assert.Equal(t, "vnc", stored.Metadata["guac.protocol"])
	assert.Equal(t, "10.0.0.5", stored.Metadata["guac.hostname"])
	assert.Equal(t, "5900", stored.Metadata["guac.port"])
	assert.Equal(t, "new", stored.Metadata["moid"])
	// 状态照常同步
	assert.Equal(t, domain.VMStatusPoweredOff, stored.Status)
}

func TestVMService_SyncVMs_MetadataNilKeepsStored(t *testing.T) {
	svc, repo, provider := newTestService()
	require.NoError(t, repo.Save(&domain.VM{
		ID: "vm-1", Name: "web",
		Metadata: map[string]any{"guac.protocol": "rdp"},
	}))
	provider.vms = []*domain.VM{{ID: "vm-1", Name: "web"}} // provider 无 metadata

	_, err := svc.SyncVMs(context.Background())
	require.NoError(t, err)

	stored, err := repo.FindByID("vm-1")
	require.NoError(t, err)
	assert.Equal(t, "rdp", stored.Metadata["guac.protocol"])
}

func TestVMService_UpdateMetadata(t *testing.T) {
	svc, repo, _ := newTestService()
	require.NoError(t, repo.Save(&domain.VM{
		ID: "vm-1", Name: "web",
		Metadata: map[string]any{"guac.protocol": "vnc"},
	}))

	vm, err := svc.UpdateMetadata(context.Background(), "vm-1", map[string]any{
		"guac.hostname": "10.0.0.9",
		"guac.protocol": "rdp",
		"guac.port":     nil, // null 删除
	})
	require.NoError(t, err)
	assert.Equal(t, "rdp", vm.Metadata["guac.protocol"])
	assert.Equal(t, "10.0.0.9", vm.Metadata["guac.hostname"])
	_, ok := vm.Metadata["guac.port"]
	assert.False(t, ok)

	stored, err := repo.FindByID("vm-1")
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.9", stored.Metadata["guac.hostname"])
}

func TestVMService_UpdateMetadata_NotFound(t *testing.T) {
	svc, _, _ := newTestService()
	_, err := svc.UpdateMetadata(context.Background(), "ghost", map[string]any{"a": "b"})
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

func TestVMService_UpdateMetadata_InitializesNil(t *testing.T) {
	svc, repo, _ := newTestService()
	require.NoError(t, repo.Save(&domain.VM{ID: "vm-1", Name: "web"}))

	vm, err := svc.UpdateMetadata(context.Background(), "vm-1", map[string]any{"guac.protocol": "ssh"})
	require.NoError(t, err)
	assert.Equal(t, "ssh", vm.Metadata["guac.protocol"])
}

func TestVMService_ListVMs(t *testing.T) {
	svc, repo, _ := newTestService()
	require.NoError(t, repo.Save(&domain.VM{ID: "vm-1", Name: "web"}))

	vms, err := svc.ListVMs(context.Background())
	require.NoError(t, err)
	assert.Len(t, vms, 1)
	assert.Equal(t, "vm-1", vms[0].ID)
}

func TestVMService_GetVM(t *testing.T) {
	svc, repo, _ := newTestService()
	require.NoError(t, repo.Save(&domain.VM{ID: "vm-1", Name: "web"}))

	vm, err := svc.GetVM(context.Background(), "vm-1")
	require.NoError(t, err)
	assert.Equal(t, "vm-1", vm.ID)

	_, err = svc.GetVM(context.Background(), "ghost")
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

func TestVMService_GetVMStatus(t *testing.T) {
	svc, repo, provider := newTestService()
	require.NoError(t, repo.Save(&domain.VM{ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOn}))
	provider.vms = []*domain.VM{{ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOff}}

	vm, err := svc.GetVMStatus(context.Background(), "vm-1")
	require.NoError(t, err)
	assert.Equal(t, domain.VMStatusPoweredOff, vm.Status)

	// 回写仓储
	stored, err := repo.FindByID("vm-1")
	require.NoError(t, err)
	assert.Equal(t, domain.VMStatusPoweredOff, stored.Status)
}

func TestVMService_GetVMStatus_MergesMetadata(t *testing.T) {
	svc, repo, provider := newTestService()
	require.NoError(t, repo.Save(&domain.VM{
		ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOn,
		Metadata: map[string]any{"guac.protocol": "vnc"},
	}))
	provider.vms = []*domain.VM{{ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOff}}

	vm, err := svc.GetVMStatus(context.Background(), "vm-1")
	require.NoError(t, err)
	assert.Equal(t, domain.VMStatusPoweredOff, vm.Status)
	// 回刷时保留手动配置的 metadata
	assert.Equal(t, "vnc", vm.Metadata["guac.protocol"])

	stored, err := repo.FindByID("vm-1")
	require.NoError(t, err)
	assert.Equal(t, "vnc", stored.Metadata["guac.protocol"])
}

func TestVMService_GetVMStatus_ProviderDown_FallsBack(t *testing.T) {
	svc, repo, provider := newTestService()
	require.NoError(t, repo.Save(&domain.VM{ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOn}))
	provider.vms = nil
	provider.err = errors.New("esxi unreachable")

	// 提供者不可达时回退仓储缓存
	vm, err := svc.GetVMStatus(context.Background(), "vm-1")
	require.NoError(t, err)
	assert.Equal(t, domain.VMStatusPoweredOn, vm.Status)

	// VM 也不在仓储中时返回 ErrNotFound
	_, err = svc.GetVMStatus(context.Background(), "ghost")
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

func TestVMService_StartVM_Success(t *testing.T) {
	svc, repo, provider := newTestService()
	require.NoError(t, repo.Save(&domain.VM{ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOff}))
	provider.vms = []*domain.VM{{ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOff}}

	vm, err := svc.StartVM(context.Background(), "vm-1")
	require.NoError(t, err)
	assert.Equal(t, domain.VMStatusPoweredOn, vm.Status)

	stored, err := repo.FindByID("vm-1")
	require.NoError(t, err)
	assert.Equal(t, domain.VMStatusPoweredOn, stored.Status)
}

func TestVMService_StartVM_AlreadyRunning(t *testing.T) {
	svc, repo, _ := newTestService()
	require.NoError(t, repo.Save(&domain.VM{ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOn}))

	_, err := svc.StartVM(context.Background(), "vm-1")
	assert.ErrorIs(t, err, ports.ErrInvalidOperation)
}

func TestVMService_StopVM_Success(t *testing.T) {
	svc, repo, provider := newTestService()
	require.NoError(t, repo.Save(&domain.VM{ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOn}))
	provider.vms = []*domain.VM{{ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOn}}

	vm, err := svc.StopVM(context.Background(), "vm-1")
	require.NoError(t, err)
	assert.Equal(t, domain.VMStatusPoweredOff, vm.Status)
}

func TestVMService_StopVM_NotRunning(t *testing.T) {
	svc, repo, _ := newTestService()
	require.NoError(t, repo.Save(&domain.VM{ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOff}))

	_, err := svc.StopVM(context.Background(), "vm-1")
	assert.ErrorIs(t, err, ports.ErrInvalidOperation)
}

func TestVMService_RestartVM_Success(t *testing.T) {
	svc, repo, provider := newTestService()
	require.NoError(t, repo.Save(&domain.VM{ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOn}))
	provider.vms = []*domain.VM{{ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOn}}

	vm, err := svc.RestartVM(context.Background(), "vm-1")
	require.NoError(t, err)
	assert.Equal(t, domain.VMStatusPoweredOn, vm.Status)
}

func TestVMService_RestartVM_NotRunning(t *testing.T) {
	svc, repo, _ := newTestService()
	require.NoError(t, repo.Save(&domain.VM{ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOff}))

	_, err := svc.RestartVM(context.Background(), "vm-1")
	assert.ErrorIs(t, err, ports.ErrInvalidOperation)
}

func TestVMService_PowerOp_NotFound(t *testing.T) {
	svc, _, _ := newTestService()

	_, err := svc.StartVM(context.Background(), "ghost")
	assert.ErrorIs(t, err, ports.ErrNotFound)
}
