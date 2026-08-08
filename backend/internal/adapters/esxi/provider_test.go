//go:build esxi

package esxi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmware/govmomi/session/keepalive"
	"github.com/vmware/govmomi/simulator"
	"github.com/vmware/govmomi/vim25/soap"
	vim "github.com/vmware/govmomi/vim25/types"

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

// TestSessionRecovery 验证会话失效（模拟 ESXi 会话超时/重启）后，
// 提供者能丢弃旧连接并自动以新会话重连，无需重启进程。
func TestSessionRecovery(t *testing.T) {
	p := newSimProvider(t)

	_, err := p.ListVMs()
	require.NoError(t, err)
	require.NotNil(t, p.client, "首次调用应建立会话")

	// 模拟 withRetry 检测到会话失效后的行为
	p.resetClient()

	_, err = p.ListVMs()
	require.NoError(t, err, "重置连接后应自动重建会话并成功")
	require.NotNil(t, p.client, "重连后应持有新的客户端")
}

// TestWithRetry_ResetsClientOnSessionError 验证 withRetry 遇到认证类错误时
// 会丢弃缓存的客户端（下次调用重连），普通错误不受影响。
func TestWithRetry_ResetsClientOnSessionError(t *testing.T) {
	p := newSimProvider(t)
	_, err := p.ListVMs()
	require.NoError(t, err)
	require.NotNil(t, p.client)

	authErr := soap.WrapVimFault(&vim.NotAuthenticated{})
	err = p.withRetry(context.Background(), "test", func() error { return authErr })
	require.Error(t, err)
	assert.Nil(t, p.client, "会话失效后应丢弃缓存的客户端，下次调用重新连接")

	p2 := newSimProvider(t)
	_, err = p2.ListVMs()
	require.NoError(t, err)
	require.NotNil(t, p2.client)
	_ = p2.withRetry(context.Background(), "test", func() error { return errors.New("普通错误") })
	assert.NotNil(t, p2.client, "普通错误不应重置客户端")
}

// TestIsSessionError 验证会话失效错误识别。
func TestIsSessionError(t *testing.T) {
	assert.True(t, isSessionError(soap.WrapVimFault(&vim.NotAuthenticated{})), "NotAuthenticated 应判定为会话失效")
	assert.True(t, isSessionError(soap.WrapVimFault(&vim.InvalidLogin{})), "InvalidLogin 应判定为会话失效")
	assert.False(t, isSessionError(errors.New("boom")), "普通错误不应判定为会话失效")
}

// TestKeepAliveStarts 验证会话保活包装已安装且心跳 goroutine 真正启动：
// 将 vcsim 会话超时设为 200ms、心跳 50ms，空闲超过超时后再次调用仍成功。
// 若心跳未启动，会话会被 vcsim 回收（NotAuthenticated），本测试会失败。
func TestKeepAliveStarts(t *testing.T) {
	old := keepAliveInterval
	keepAliveInterval = 50 * time.Millisecond
	defer func() { keepAliveInterval = old }()

	model := simulator.VPX()
	require.NoError(t, model.Create())
	defer model.Remove()

	s := model.Service
	s.Context.Map.OptionManager().Setting = append(s.Context.Map.OptionManager().Setting,
		&vim.OptionValue{Key: "config.vmacore.soap.sessionTimeout", Value: "200ms"})

	srv := s.NewServer()
	defer srv.Close()

	p := NewProvider(Config{
		URL:      srv.URL.String(),
		Username: "user",
		Password: "pass",
		Insecure: true,
	})
	t.Cleanup(func() { _ = p.Close() })

	_, err := p.ListVMs()
	require.NoError(t, err, "首次调用应成功")
	_, ok := p.client.Client.RoundTripper.(*keepalive.HandlerSOAP)
	require.True(t, ok, "应安装 keepalive 会话保活包装")

	time.Sleep(500 * time.Millisecond)
	_, err = p.ListVMs()
	require.NoError(t, err, "心跳保活后会话应仍有效")
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

// TestMapPowerState 验证状态映射（全部枚举均可映射，未知值回退 Unknown）。
func TestMapPowerState(t *testing.T) {
	assert.Equal(t, domain.VMStatusPoweredOn, mapPowerState(vim.VirtualMachinePowerStatePoweredOn))
	assert.Equal(t, domain.VMStatusPoweredOff, mapPowerState(vim.VirtualMachinePowerStatePoweredOff))
	assert.Equal(t, domain.VMStatusSuspended, mapPowerState(vim.VirtualMachinePowerStateSuspended))
	assert.Equal(t, domain.VMStatusUnknown, mapPowerState(vim.VirtualMachinePowerState("weird")))
}
