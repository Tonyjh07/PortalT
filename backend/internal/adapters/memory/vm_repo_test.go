package memory

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

// testVM 构造测试虚拟机
func testVM(id string) *domain.VM {
	return &domain.VM{
		ID:        id,
		Name:      "vm-" + id,
		Status:    domain.VMStatusPoweredOn,
		CPU:       2,
		MemoryMB:  4096,
		IPAddress: "192.168.1.10",
		Host:      "esxi-01",
	}
}

func TestVMRepository_SaveAndFindByID(t *testing.T) {
	repo := NewVMRepository()
	vm := testVM("vm-1")

	require.NoError(t, repo.Save(vm))

	got, err := repo.FindByID("vm-1")
	require.NoError(t, err)
	assert.Equal(t, vm, got)
}

func TestVMRepository_Save_Upsert(t *testing.T) {
	repo := NewVMRepository()
	require.NoError(t, repo.Save(testVM("vm-1")))

	updated := testVM("vm-1")
	updated.Status = domain.VMStatusPoweredOff
	require.NoError(t, repo.Save(updated))

	got, err := repo.FindByID("vm-1")
	require.NoError(t, err)
	assert.Equal(t, domain.VMStatusPoweredOff, got.Status)
	assert.Equal(t, 1, len(repo.vms))
}

func TestVMRepository_FindByID_NotFound(t *testing.T) {
	repo := NewVMRepository()
	got, err := repo.FindByID("missing")
	assert.ErrorIs(t, err, ports.ErrNotFound)
	assert.Nil(t, got)
}

func TestVMRepository_FindAll(t *testing.T) {
	repo := NewVMRepository()
	require.NoError(t, repo.Save(testVM("vm-1")))
	require.NoError(t, repo.Save(testVM("vm-2")))

	vms, err := repo.FindAll()
	require.NoError(t, err)
	assert.Len(t, vms, 2)
}

func TestVMRepository_FindAll_Empty(t *testing.T) {
	repo := NewVMRepository()
	vms, err := repo.FindAll()
	require.NoError(t, err)
	assert.NotNil(t, vms)
	assert.Empty(t, vms)
}

func TestVMRepository_Delete(t *testing.T) {
	repo := NewVMRepository()
	require.NoError(t, repo.Save(testVM("vm-1")))

	require.NoError(t, repo.Delete("vm-1"))
	_, err := repo.FindByID("vm-1")
	assert.ErrorIs(t, err, ports.ErrNotFound)

	err = repo.Delete("vm-1")
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

func TestVMRepository_Delete_NotFound(t *testing.T) {
	repo := NewVMRepository()
	err := repo.Delete("missing")
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

func TestVMRepository_Save_Invalid(t *testing.T) {
	repo := NewVMRepository()
	assert.ErrorIs(t, repo.Save(nil), ports.ErrInvalidArgument)
	assert.ErrorIs(t, repo.Save(&domain.VM{}), ports.ErrInvalidArgument)
}

func TestVMRepository_ConcurrentAccess(t *testing.T) {
	repo := NewVMRepository()
	const n = 100

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "vm-" + string(rune('a'+i%26)) + string(rune('0'+i/26))
			_ = repo.Save(testVM(id))
			_, _ = repo.FindByID(id)
		}(i)
	}
	wg.Wait()

	vms, err := repo.FindAll()
	require.NoError(t, err)
	assert.Len(t, vms, 100)
}

// 编译期断言：内存仓储实现 ports 接口
var _ ports.VMRepository = (*VMRepository)(nil)
