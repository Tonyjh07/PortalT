package gormstore

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

func TestVMRepository_Crud(t *testing.T) {
	db := newTestDB(t)
	repo := NewVMRepository(db)

	vm := &domain.VM{
		ID:        "vm-1",
		Name:      "web-server",
		Status:    domain.VMStatusPoweredOn,
		CPU:       4,
		MemoryMB:  8192,
		IPAddress: "192.168.1.10",
		Host:      "esxi-01",
	}

	require.NoError(t, repo.Save(vm))
	got, err := repo.FindByID("vm-1")
	require.NoError(t, err)
	assert.Equal(t, vm, got)

	// upsert
	vm.Status = domain.VMStatusPoweredOff
	require.NoError(t, repo.Save(vm))
	got, err = repo.FindByID("vm-1")
	require.NoError(t, err)
	assert.Equal(t, domain.VMStatusPoweredOff, got.Status)

	require.NoError(t, repo.Delete("vm-1"))
	_, err = repo.FindByID("vm-1")
	assert.ErrorIs(t, err, ports.ErrNotFound)
	assert.ErrorIs(t, repo.Delete("vm-1"), ports.ErrNotFound)
}

func TestVMRepository_MetadataRoundtrip(t *testing.T) {
	db := newTestDB(t)
	repo := NewVMRepository(db)

	vm := &domain.VM{
		ID:       "vm-2",
		Name:     "db",
		Status:   domain.VMStatusPoweredOn,
		CPU:      2,
		MemoryMB: 4096,
		Metadata: map[string]any{
			"proto": "rdp",
			"port":  float64(3389),
			"tags":  []any{"prod", "db"},
		},
	}
	require.NoError(t, repo.Save(vm))

	got, err := repo.FindByID("vm-2")
	require.NoError(t, err)
	assert.Equal(t, "rdp", got.Metadata["proto"])
	assert.Equal(t, float64(3389), got.Metadata["port"])

	// 未设置时归一为 nil
	plain := &domain.VM{ID: "vm-3", Name: "plain"}
	require.NoError(t, repo.Save(plain))
	got, err = repo.FindByID("vm-3")
	require.NoError(t, err)
	assert.Nil(t, got.Metadata)
}

func TestVMRepository_FindAll_Ordered(t *testing.T) {
	db := newTestDB(t)
	repo := NewVMRepository(db)
	require.NoError(t, repo.Save(&domain.VM{ID: "vm-b", Name: "b"}))
	require.NoError(t, repo.Save(&domain.VM{ID: "vm-a", Name: "a"}))

	vms, err := repo.FindAll()
	require.NoError(t, err)
	require.Len(t, vms, 2)
	assert.Equal(t, "a", vms[0].Name)
	assert.Equal(t, "b", vms[1].Name)
}

func TestVMRepository_Errors(t *testing.T) {
	db := newTestDB(t)
	repo := NewVMRepository(db)

	_, err := repo.FindByID("missing")
	assert.ErrorIs(t, err, ports.ErrNotFound)
	assert.ErrorIs(t, repo.Save(nil), ports.ErrInvalidArgument)
	assert.ErrorIs(t, repo.Save(&domain.VM{}), ports.ErrInvalidArgument)
}

func TestVMRepository_ConcurrentSave(t *testing.T) {
	db := newTestDB(t)
	repo := NewVMRepository(db)

	const n = 20
	done := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			vm := &domain.VM{ID: "vm-x", Name: "x", Status: domain.VMStatusPoweredOn}
			vm.CPU = i
			done <- repo.Save(vm)
		}(i)
	}
	for i := 0; i < n; i++ {
		require.NoError(t, <-done, "并发 upsert 不应失败")
	}

	got, err := repo.FindByID("vm-x")
	require.NoError(t, err)
	assert.Equal(t, "x", got.Name)
}

func TestVMRepository_ConcurrentReadWrite(t *testing.T) {
	db := newTestDB(t)
	repo := NewVMRepository(db)
	require.NoError(t, repo.Save(&domain.VM{ID: "vm-1", Name: "x"}))

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				_, _ = repo.FindAll()
			} else {
				_ = repo.Save(&domain.VM{ID: "vm-1", Name: "x", CPU: i})
			}
		}(i)
	}
	wg.Wait()
}
