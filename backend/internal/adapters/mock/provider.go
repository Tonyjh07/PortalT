// Package mock 提供内存态虚拟化平台模拟器，用于开发调试与测试。
// 通过 VIRT_PROVIDER=mock 切换，无需真实 ESXi 环境。
package mock

import (
	"fmt"
	"sync"

	"portalt/internal/domain"
)

// Provider 内存态虚拟化平台提供者，内置示例虚拟机与宿主机。
type Provider struct {
	mu   sync.RWMutex
	vms  map[string]*domain.VM
	host *domain.HostInfo
}

// NewProvider 创建带示例数据的 Mock 提供者。
// 配置键当前仅用于占位，保持与 esxi 工厂参数一致。
func NewProvider(_ map[string]string) *Provider {
	p := &Provider{
		vms: make(map[string]*domain.VM),
		host: &domain.HostInfo{
			Name:          "mock-host",
			Version:       "1.0.0-mock",
			CPUModel:      "Mock CPU 2.0GHz",
			TotalCPU:      8,
			UsedCPU:       2,
			TotalMemoryMB: 16384,
			UsedMemoryMB:  4096,
			Status:        "connected",
		},
	}
	for i := 1; i <= 3; i++ {
		id := fmt.Sprintf("vm-mock-%d", i)
		p.vms[id] = &domain.VM{
			ID:        id,
			Name:      fmt.Sprintf("mock-vm-%d", i),
			Status:    domain.VMStatusPoweredOn,
			CPU:       2,
			MemoryMB:  2048,
			Host:      "mock-host",
			IPAddress: fmt.Sprintf("192.168.1.%d", i),
			// guac.* 为远程桌面连接参数（Phase 8），隧道建立时注入 guacd，
			// 浏览器侧无法覆盖。演示环境：guacd（容器）经 host.docker.internal
			// 访问宿主机 5900 端口的 VNC 演示容器（见 docker-compose）。
			Metadata: map[string]any{
				"guac.protocol": "vnc",
				"guac.hostname": "host.docker.internal",
				"guac.port":     "5900",
				"guac.password": "portalt-demo",
			},
		}
	}
	return p
}

// SetVMs 替换全部虚拟机（测试控制用）。
func (p *Provider) SetVMs(vms []*domain.VM) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.vms = make(map[string]*domain.VM, len(vms))
	for _, v := range vms {
		p.vms[v.ID] = cloneVM(v)
	}
}

// SetHostInfo 替换宿主机信息（测试控制用）。
func (p *Provider) SetHostInfo(h *domain.HostInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := *h
	p.host = &cp
}

// ListVMs 返回全部虚拟机快照。
func (p *Provider) ListVMs() ([]*domain.VM, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]*domain.VM, 0, len(p.vms))
	for _, v := range p.vms {
		out = append(out, cloneVM(v))
	}
	return out, nil
}

// StartVM 将虚拟机置为开机状态。
func (p *Provider) StartVM(id string) error {
	return p.setStatus(id, domain.VMStatusPoweredOn)
}

// StopVM 将虚拟机置为关机状态。
func (p *Provider) StopVM(id string) error {
	return p.setStatus(id, domain.VMStatusPoweredOff)
}

// RestartVM 将虚拟机置为开机状态。
func (p *Provider) RestartVM(id string) error {
	return p.setStatus(id, domain.VMStatusPoweredOn)
}

// GetHostInfo 返回宿主机信息快照。
func (p *Provider) GetHostInfo() (*domain.HostInfo, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	cp := *p.host
	return &cp, nil
}

func (p *Provider) setStatus(id string, s domain.VMStatus) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	v, ok := p.vms[id]
	if !ok {
		return fmt.Errorf("mock: 虚拟机 %q 不存在", id)
	}
	v.Status = s
	return nil
}

func cloneVM(v *domain.VM) *domain.VM {
	cp := *v
	return &cp
}
