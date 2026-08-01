//go:build esxi

package esxi

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/simulator"

	"portalt/internal/domain"
)

// newSimProvider 启动 vcsim 模拟 vCenter 并返回已连接的提供者。
func newSimProvider(t *testing.T) *Provider {
	t.Helper()
	model := simulator.VPX()
	require.NoError(t, model.Create())
	srv := model.Service.NewServer()
	t.Cleanup(srv.Close)

	p := NewProvider(Config{
		URL:      srv.URL.String(),
		Username: "user",
		Password: "pass",
		Insecure: true,
	})
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func TestListVMs(t *testing.T) {
	p := newSimProvider(t)

	vms, err := p.ListVMs()
	require.NoError(t, err)
	require.NotEmpty(t, vms, "vcsim VPX 模型应包含虚拟机")

	for _, vm := range vms {
		assert.NotEmpty(t, vm.ID, "虚拟机ID不能为空")
		assert.NotEmpty(t, vm.Name, "虚拟机名称不能为空")
		assert.Greater(t, vm.CPU, 0, "CPU核数应大于0")
		assert.Greater(t, vm.MemoryMB, 0, "内存应大于0")
		assert.Contains(t, []domain.VMStatus{
			domain.VMStatusPoweredOn,
			domain.VMStatusPoweredOff,
			domain.VMStatusSuspended,
			domain.VMStatusUnknown,
		}, vm.Status)
		assert.NotEmpty(t, vm.Metadata["moid"], "应保留平台 MOID")
	}
}

func TestListVMs_HostResolved(t *testing.T) {
	p := newSimProvider(t)

	vms, err := p.ListVMs()
	require.NoError(t, err)

	hosted := false
	for _, vm := range vms {
		if vm.Host != "" {
			hosted = true
			break
		}
	}
	assert.True(t, hosted, "至少一台虚拟机应解析出宿主机名")
}

func TestPowerOps(t *testing.T) {
	p := newSimProvider(t)

	vms, err := p.ListVMs()
	require.NoError(t, err)
	target := vms[0]

	require.NoError(t, p.StopVM(target.ID))
	after, err := p.ListVMs()
	require.NoError(t, err)
	assert.Equal(t, domain.VMStatusPoweredOff, findVM(t, after, target.ID).Status, "关机后状态应为 poweredOff")

	require.NoError(t, p.StartVM(target.ID))
	after, err = p.ListVMs()
	require.NoError(t, err)
	assert.Equal(t, domain.VMStatusPoweredOn, findVM(t, after, target.ID).Status, "开机后状态应为 poweredOn")

	require.NoError(t, p.RestartVM(target.ID))
	after, err = p.ListVMs()
	require.NoError(t, err)
	assert.Equal(t, domain.VMStatusPoweredOn, findVM(t, after, target.ID).Status, "重启后状态应为 poweredOn")
}

func TestPowerOp_NotFound(t *testing.T) {
	p := newSimProvider(t)
	assert.Error(t, p.StopVM("vm-doesnotexist"))
}

func TestGetHostInfo(t *testing.T) {
	p := newSimProvider(t)

	info, err := p.GetHostInfo()
	require.NoError(t, err)
	assert.NotEmpty(t, info.Name)
	assert.NotEmpty(t, info.Version, "应包含平台版本")
	assert.NotEmpty(t, info.CPUModel)
	assert.Greater(t, info.TotalCPU, 0)
	assert.Greater(t, info.TotalMemoryMB, 0)
	assert.Equal(t, "connected", info.Status)
}

func TestConnectionFailure(t *testing.T) {
	p := NewProvider(Config{
		URL:      "https://127.0.0.1:1/sdk",
		Username: "user",
		Password: "pass",
		Insecure: true,
		Timeout:  2 * time.Second,
	})
	t.Cleanup(func() { _ = p.Close() })

	_, err := p.ListVMs()
	assert.Error(t, err)
}

func TestRetryRecovers(t *testing.T) {
	p := newSimProvider(t)

	// 首次调用触发连接后，重复调用应复用会话而非报错
	vms, err := p.ListVMs()
	require.NoError(t, err)
	_, err = p.ListVMs()
	require.NoError(t, err)
	assert.NotEmpty(t, vms)
}

func findVM(t *testing.T, vms []*domain.VM, id string) *domain.VM {
	t.Helper()
	for _, vm := range vms {
		if vm.ID == id {
			return vm
		}
	}
	t.Fatalf("虚拟机 %s 未找到", id)
	return nil
}
