package mock

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/domain"
)

func TestProvider_SeededInventory(t *testing.T) {
	p := NewProvider(nil)

	vms, err := p.ListVMs()
	require.NoError(t, err)
	assert.Len(t, vms, 3, "默认应内置3台示例虚拟机")

	ids := make(map[string]bool)
	for _, vm := range vms {
		assert.NotEmpty(t, vm.ID)
		assert.NotEmpty(t, vm.Name)
		assert.False(t, ids[vm.ID], "虚拟机ID不应重复")
		ids[vm.ID] = true
	}
}

func TestProvider_ListVMs_ReturnsCopy(t *testing.T) {
	p := NewProvider(nil)

	vms, err := p.ListVMs()
	require.NoError(t, err)
	vms[0].Status = domain.VMStatusPoweredOff

	again, err := p.ListVMs()
	require.NoError(t, err)
	assert.Equal(t, domain.VMStatusPoweredOn, again[0].Status, "外部修改不应影响内部状态")
}

func TestProvider_PowerOps(t *testing.T) {
	p := NewProvider(nil)

	require.NoError(t, p.StopVM("vm-mock-1"))
	vms, err := p.ListVMs()
	require.NoError(t, err)
	assert.Equal(t, domain.VMStatusPoweredOff, mockVM(vms, "vm-mock-1").Status)

	require.NoError(t, p.StartVM("vm-mock-1"))
	require.NoError(t, p.RestartVM("vm-mock-1"))
	vms, err = p.ListVMs()
	require.NoError(t, err)
	assert.Equal(t, domain.VMStatusPoweredOn, mockVM(vms, "vm-mock-1").Status)
}

func mockVM(vms []*domain.VM, id string) *domain.VM {
	for _, vm := range vms {
		if vm.ID == id {
			return vm
		}
	}
	return nil
}

func TestProvider_PowerOp_NotFound(t *testing.T) {
	p := NewProvider(nil)
	assert.Error(t, p.StopVM("vm-ghost"))
	assert.Error(t, p.StartVM(""))
}

func TestProvider_GetHostInfo(t *testing.T) {
	p := NewProvider(nil)

	info, err := p.GetHostInfo()
	require.NoError(t, err)
	assert.Equal(t, "connected", info.Status)
	assert.Greater(t, info.TotalCPU, 0)

	info.Name = "mutated"
	again, err := p.GetHostInfo()
	require.NoError(t, err)
	assert.NotEqual(t, "mutated", again.Name, "外部修改不应影响内部状态")
}

func TestProvider_SetVMs(t *testing.T) {
	p := NewProvider(nil)
	p.SetVMs([]*domain.VM{{ID: "custom-1", Name: "custom", Status: domain.VMStatusPoweredOff}})

	vms, err := p.ListVMs()
	require.NoError(t, err)
	require.Len(t, vms, 1)
	assert.Equal(t, "custom-1", vms[0].ID)
	assert.Equal(t, "custom", vms[0].Name)
}

func TestProvider_ConcurrentOps(t *testing.T) {
	p := NewProvider(nil)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if n%2 == 0 {
				_ = p.StartVM("vm-mock-1")
			} else {
				_, _ = p.ListVMs()
			}
		}(i)
	}
	wg.Wait()

	vms, err := p.ListVMs()
	require.NoError(t, err)
	require.Len(t, vms, 3)
}
