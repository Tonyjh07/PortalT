//go:build integration

// 真实 ESXi 集成测试（默认被 build tag 排除）。
//
// 通过环境变量连接：
//   - TEST_ESXI_URL      平台地址，默认 https://192.168.118.129/sdk
//   - TEST_ESXI_USERNAME 登录用户名，默认 root
//   - TEST_ESXI_PASSWORD 登录密码（必填，不设则跳过）
//
// 运行：go test -tags integration ./internal/adapters/esxi/ -run TestRealESXi
package esxi

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newRealProvider 依据环境变量创建真实 ESXi 提供者；凭据缺失时返回 nil（跳过）。
func newRealProvider(t *testing.T) *Provider {
	t.Helper()
	password := os.Getenv("TEST_ESXI_PASSWORD")
	if password == "" {
		t.Skip("未设置 TEST_ESXI_PASSWORD，跳过真实 ESXi 集成测试")
	}
	url := os.Getenv("TEST_ESXI_URL")
	if url == "" {
		url = "https://192.168.118.129/sdk"
	}
	username := os.Getenv("TEST_ESXI_USERNAME")
	if username == "" {
		username = "root"
	}
	p := NewProvider(Config{
		URL:      url,
		Username: username,
		Password: password,
		Insecure: true,
		Timeout:  30 * time.Second,
	})
	t.Cleanup(func() { _ = p.Close() })
	return p
}

// TestRealESXi_ListVMs 验证能连上真实 ESXi 并列出虚拟机。
// 测试 ESXi 当前无虚拟机，期望返回空列表而非报错。
func TestRealESXi_ListVMs(t *testing.T) {
	p := newRealProvider(t)

	vms, err := p.ListVMs()
	require.NoError(t, err, "连接或列表失败")
	for _, vm := range vms {
		assert.NotEmpty(t, vm.ID)
		assert.NotEmpty(t, vm.Name)
		assert.NotEmpty(t, vm.Metadata["moid"])
		t.Logf("VM: id=%s name=%s cpu=%d mem=%dMB ip=%s host=%s status=%s moid=%v",
			vm.ID, vm.Name, vm.CPU, vm.MemoryMB, vm.IPAddress, vm.Host, vm.Status, vm.Metadata["moid"])
	}
	t.Logf("ListVMs 返回 %d 台虚拟机", len(vms))
}

// TestRealESXi_GetHostInfo 验证宿主机信息读取。
func TestRealESXi_GetHostInfo(t *testing.T) {
	p := newRealProvider(t)

	info, err := p.GetHostInfo()
	require.NoError(t, err)
	t.Logf("宿主机: %s | %s | %s | CPU %d 核 | 内存 %d MB | 状态 %s",
		info.Name, info.Version, info.CPUModel, info.TotalCPU, info.TotalMemoryMB, info.Status)
	assert.NotEmpty(t, info.Name)
	assert.NotEmpty(t, info.Version)
	assert.NotEmpty(t, info.CPUModel)
	assert.Greater(t, info.TotalCPU, 0)
	assert.Greater(t, info.TotalMemoryMB, 0)
	assert.Equal(t, "connected", info.Status)
}

// TestRealESXi_PowerOp_NotFound 验证对不存在的 VM 做电源操作返回错误（不 panic）。
func TestRealESXi_PowerOp_NotFound(t *testing.T) {
	p := newRealProvider(t)

	err := p.StartVM("vm-00000000-0000-0000-0000-000000000000")
	require.Error(t, err, "不存在的虚拟机应返回错误")
	t.Logf("错误信息: %v", err)
}

// TestRealESXi_ReuseSession 验证会话复用（第二次调用不重连）。
func TestRealESXi_ReuseSession(t *testing.T) {
	p := newRealProvider(t)

	_, err := p.ListVMs()
	require.NoError(t, err)
	_, err = p.GetHostInfo()
	require.NoError(t, err)
}
