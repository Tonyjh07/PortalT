// Package esxi 实现 VMware ESXi / vCenter 虚拟化平台提供者。
//
// 通过 github.com/vmware/govmomi 与平台 SOAP API 交互，
// 使用 property collector 批量拉取虚拟机信息；
// 对网络抖动等瞬时故障采用指数退避重试。
package esxi

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25/mo"
	vim "github.com/vmware/govmomi/vim25/types"

	"portalt/internal/domain"
)

// Config ESXi 平台连接配置。
type Config struct {
	// URL 平台地址，如 https://esxi.lan/sdk
	URL string
	// Username 登录用户名
	Username string
	// Password 登录密码
	Password string
	// Insecure 跳过TLS证书校验（自签证书环境）
	Insecure bool
	// Timeout 单次操作超时，默认 30s
	Timeout time.Duration
	// MaxRetries 瞬时故障重试次数，默认 3
	MaxRetries int
}

// withDefaults 补齐默认配置。
func (c Config) withDefaults() Config {
	if c.Timeout <= 0 {
		c.Timeout = 30 * time.Second
	}
	if c.MaxRetries <= 0 {
		c.MaxRetries = 3
	}
	return c
}

// Provider ESXi 虚拟化平台提供者（惰性连接，首次调用时建立）。
type Provider struct {
	cfg Config

	mu     sync.Mutex
	client *govmomi.Client
}

// NewProvider 创建 ESXi 提供者。连接延迟到首次调用建立，
// 便于工厂在配置阶段快速创建。
func NewProvider(cfg Config) *Provider {
	return &Provider{cfg: cfg.withDefaults()}
}

// Close 登出并关闭连接。重复调用安全。
func (p *Provider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := p.client.Logout(ctx)
	p.client = nil
	return err
}

// ListVMs 返回平台上的全部虚拟机（含主机名解析）。
func (p *Provider) ListVMs() ([]*domain.VM, error) {
	ctx, cancel := p.ctx()
	defer cancel()
	c, err := p.ensureClient(ctx)
	if err != nil {
		return nil, err
	}

	var vms []*domain.VM
	err = p.withRetry(ctx, "ListVMs", func() error {
		v, err := p.newContainerView(ctx, c, "VirtualMachine")
		if err != nil {
			return err
		}
		defer func() { _ = v.Destroy(ctx) }()

		var moVMs []mo.VirtualMachine
		if err := v.Retrieve(ctx, []string{"VirtualMachine"}, []string{"summary", "guest", "config"}, &moVMs); err != nil {
			return err
		}

		hostNames, err := p.hostNames(ctx, c)
		if err != nil {
			return err
		}

		vms = make([]*domain.VM, 0, len(moVMs))
		for i := range moVMs {
			vms = append(vms, toDomainVM(&moVMs[i], hostNames))
		}
		return nil
	})
	return vms, err
}

// StartVM 启动指定虚拟机。
func (p *Provider) StartVM(id string) error {
	return p.powerOp(id, "启动", func(ctx context.Context, vm *object.VirtualMachine) error {
		t, err := vm.PowerOn(ctx)
		if err != nil {
			return err
		}
		return t.Wait(ctx)
	})
}

// StopVM 强制关机指定虚拟机。
func (p *Provider) StopVM(id string) error {
	return p.powerOp(id, "关机", func(ctx context.Context, vm *object.VirtualMachine) error {
		t, err := vm.PowerOff(ctx)
		if err != nil {
			return err
		}
		return t.Wait(ctx)
	})
}

// RestartVM 重启指定虚拟机。
func (p *Provider) RestartVM(id string) error {
	return p.powerOp(id, "重启", func(ctx context.Context, vm *object.VirtualMachine) error {
		t, err := vm.Reset(ctx)
		if err != nil {
			return err
		}
		return t.Wait(ctx)
	})
}

// GetHostInfo 返回第一台宿主机信息（ESXi 单机即唯一宿主）。
func (p *Provider) GetHostInfo() (*domain.HostInfo, error) {
	ctx, cancel := p.ctx()
	defer cancel()
	c, err := p.ensureClient(ctx)
	if err != nil {
		return nil, err
	}

	var info *domain.HostInfo
	err = p.withRetry(ctx, "GetHostInfo", func() error {
		v, err := p.newContainerView(ctx, c, "HostSystem")
		if err != nil {
			return err
		}
		defer func() { _ = v.Destroy(ctx) }()

		var hosts []mo.HostSystem
		if err := v.Retrieve(ctx, []string{"HostSystem"}, []string{"name", "summary", "runtime"}, &hosts); err != nil {
			return err
		}

		for i := range hosts {
			info = toHostInfo(&hosts[i])
			return nil
		}
		return errors.New("esxi: 未发现宿主机")
	})
	return info, err
}

// ensureClient 建立（或复用）与平台的会话。
func (p *Provider) ensureClient(ctx context.Context) (*govmomi.Client, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.client != nil {
		return p.client, nil
	}

	u, err := url.Parse(p.cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("esxi: 无效URL %q: %w", p.cfg.URL, err)
	}
	u.User = url.UserPassword(p.cfg.Username, p.cfg.Password)

	c, err := govmomi.NewClient(ctx, u, p.cfg.Insecure)
	if err != nil {
		return nil, fmt.Errorf("esxi: 连接 %s 失败: %w", p.cfg.URL, err)
	}
	p.client = c
	return c, nil
}

// ctx 基于后台上下文，附加单次操作超时。
func (p *Provider) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), p.cfg.Timeout)
}

// newContainerView 创建根目录下的递归容器视图。
func (p *Provider) newContainerView(ctx context.Context, c *govmomi.Client, kind string) (*view.ContainerView, error) {
	m := view.NewManager(c.Client)
	return m.CreateContainerView(ctx, c.ServiceContent.RootFolder, []string{kind}, true)
}

// hostNames 拉取全部宿主机的 MOID → 主机名映射。
func (p *Provider) hostNames(ctx context.Context, c *govmomi.Client) (map[string]string, error) {
	v, err := p.newContainerView(ctx, c, "HostSystem")
	if err != nil {
		return nil, err
	}
	defer func() { _ = v.Destroy(ctx) }()

	var hosts []mo.HostSystem
	if err := v.Retrieve(ctx, []string{"HostSystem"}, []string{"name"}, &hosts); err != nil {
		return nil, err
	}
	names := make(map[string]string, len(hosts))
	for i := range hosts {
		names[hosts[i].Self.Value] = hosts[i].Name
	}
	return names, nil
}

// powerOp 查找虚拟机并执行电源操作，操作带重试。
func (p *Provider) powerOp(id, opName string, op func(ctx context.Context, vm *object.VirtualMachine) error) error {
	ctx, cancel := p.ctx()
	defer cancel()
	c, err := p.ensureClient(ctx)
	if err != nil {
		return err
	}

	return p.withRetry(ctx, opName, func() error {
		vm, err := p.findVM(ctx, c, id)
		if err != nil {
			return err
		}
		return op(ctx, vm)
	})
}

// findVM 按 ID 定位虚拟机：支持 MOID（vm- 前缀）与 UUID 两种标识。
func (p *Provider) findVM(ctx context.Context, c *govmomi.Client, id string) (*object.VirtualMachine, error) {
	if strings.HasPrefix(id, "vm-") {
		return object.NewVirtualMachine(c.Client, vim.ManagedObjectReference{
			Type:  "VirtualMachine",
			Value: id,
		}), nil
	}

	si := object.NewSearchIndex(c.Client)
	ref, err := si.FindByUuid(ctx, nil, id, true, nil)
	if err != nil {
		return nil, fmt.Errorf("esxi: 查找虚拟机 %q: %w", id, err)
	}
	if ref == nil {
		return nil, fmt.Errorf("esxi: 虚拟机 %q 不存在", id)
	}
	return object.NewVirtualMachine(c.Client, ref.Reference()), nil
}

// withRetry 指数退避重试（200ms、400ms、800ms…），上下文取消立即返回。
func (p *Provider) withRetry(ctx context.Context, opName string, fn func() error) error {
	var lastErr error
	for attempt := 0; attempt < p.cfg.MaxRetries; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt+1 < p.cfg.MaxRetries {
			delay := time.Duration(1<<attempt) * 200 * time.Millisecond
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return fmt.Errorf("esxi: %s 重试 %d 次仍失败: %w", opName, p.cfg.MaxRetries, lastErr)
}

// toDomainVM 将平台虚拟机对象映射为领域实体。
func toDomainVM(m *mo.VirtualMachine, hostNames map[string]string) *domain.VM {
	vm := &domain.VM{
		ID:     m.Self.Value,
		Status: mapPowerState(m.Summary.Runtime.PowerState),
	}
	vm.ID = m.Summary.Config.Uuid
	vm.Name = m.Summary.Config.Name
	vm.CPU = int(m.Summary.Config.NumCpu)
	vm.MemoryMB = int(m.Summary.Config.MemorySizeMB)
	if m.Guest != nil {
		vm.IPAddress = m.Guest.IpAddress
	}
	if m.Summary.Runtime.Host != nil {
		vm.Host = hostNames[m.Summary.Runtime.Host.Value]
	}
	// 保留 MOID 供控制台/扩展使用；UUID 作为稳定 ID
	vm.Metadata = map[string]any{"moid": m.Self.Value}
	return vm
}

// mapPowerState 平台电源状态 → 领域状态。
func mapPowerState(s vim.VirtualMachinePowerState) domain.VMStatus {
	switch s {
	case vim.VirtualMachinePowerStatePoweredOn:
		return domain.VMStatusPoweredOn
	case vim.VirtualMachinePowerStatePoweredOff:
		return domain.VMStatusPoweredOff
	case vim.VirtualMachinePowerStateSuspended:
		return domain.VMStatusSuspended
	default:
		return domain.VMStatusUnknown
	}
}

// toHostInfo 将平台宿主机对象映射为领域实体。
func toHostInfo(h *mo.HostSystem) *domain.HostInfo {
	info := &domain.HostInfo{
		Name:   h.Name,
		Status: "connected",
	}
	if h.Summary.Config.Product != nil {
		info.Version = h.Summary.Config.Product.FullName
	}
	if hw := h.Summary.Hardware; hw != nil {
		info.CPUModel = hw.CpuModel
		info.TotalCPU = int(hw.NumCpuCores)
		if totalMHz := int64(hw.CpuMhz) * int64(hw.NumCpuCores); totalMHz > 0 {
			// OverallCpuUsage 为 MHz，换算为核数
			info.UsedCPU = int(float64(h.Summary.QuickStats.OverallCpuUsage)*float64(hw.NumCpuCores)/float64(totalMHz) + 0.5)
		}
		info.TotalMemoryMB = int(hw.MemorySize / 1024 / 1024)
		info.UsedMemoryMB = int(h.Summary.QuickStats.OverallMemoryUsage)
	}
	if h.Summary.Runtime != nil && h.Summary.Runtime.ConnectionState != vim.HostSystemConnectionStateConnected {
		info.Status = "disconnected"
	}
	return info
}
