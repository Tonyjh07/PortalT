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

func (s *stubProvider) StartVM(id string) error   { return nil }
func (s *stubProvider) StopVM(id string) error    { return nil }
func (s *stubProvider) RestartVM(id string) error { return nil }

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

func TestVMService_ListVMs(t *testing.T) {
	svc, repo, _ := newTestService()
	require.NoError(t, repo.Save(&domain.VM{ID: "vm-1", Name: "web"}))

	vms, err := svc.ListVMs(context.Background())
	require.NoError(t, err)
	assert.Len(t, vms, 1)
	assert.Equal(t, "vm-1", vms[0].ID)
}
