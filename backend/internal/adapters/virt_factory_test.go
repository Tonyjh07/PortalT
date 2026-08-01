package adapters

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/adapters/esxi"
	"portalt/internal/adapters/mock"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

// 编译期断言：适配器实现端口接口。
var (
	_ ports.VirtualizationProvider = (*mock.Provider)(nil)
	_ ports.VirtualizationProvider = (*esxi.Provider)(nil)
)

func TestNewVirtualizationProvider_Mock(t *testing.T) {
	for _, typ := range []string{"", "mock", "MOCK"} {
		p, err := NewVirtualizationProvider(typ, nil)
		require.NoError(t, err)

		vms, err := p.ListVMs()
		require.NoError(t, err)
		assert.NotEmpty(t, vms, "mock 提供者应内置示例虚拟机")
	}
}

func TestNewVirtualizationProvider_ESXi_MissingURL(t *testing.T) {
	_, err := NewVirtualizationProvider("esxi", map[string]string{"username": "root"})
	assert.ErrorContains(t, err, "url")
}

func TestNewVirtualizationProvider_ESXi_LazyConnect(t *testing.T) {
	// 工厂创建不触发连接，仅校验配置
	p, err := NewVirtualizationProvider("esxi", map[string]string{
		"url":      "https://esxi.example.com/sdk",
		"username": "root",
		"password": "secret",
		"insecure": "true",
	})
	require.NoError(t, err)

	vms, err := p.ListVMs()
	assert.Error(t, err, "地址不可达时应报连接错误")
	assert.Nil(t, vms)
}

func TestNewVirtualizationProvider_Unknown(t *testing.T) {
	_, err := NewVirtualizationProvider("proxmox", nil)
	assert.ErrorContains(t, err, "proxmox")
}

func TestFactory_MockHostInfo(t *testing.T) {
	p, err := NewVirtualizationProvider("mock", nil)
	require.NoError(t, err)

	info, err := p.GetHostInfo()
	require.NoError(t, err)
	assert.Equal(t, "connected", info.Status)
	assert.IsType(t, &domain.HostInfo{}, info)
}
