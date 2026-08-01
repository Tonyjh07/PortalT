package ports

import "portalt/internal/domain"

// VirtualizationProvider 虚拟化平台提供者接口。
// 由 adapters 层实现（esxi/proxmox/mock），
// 领域服务通过本接口操作虚拟机，实现平台可移植性。
type VirtualizationProvider interface {
	// ListVMs 获取平台上的全部虚拟机
	ListVMs() ([]*domain.VM, error)
	// StartVM 启动指定虚拟机
	StartVM(id string) error
	// StopVM 停止指定虚拟机
	StopVM(id string) error
	// RestartVM 重启指定虚拟机
	RestartVM(id string) error
	// GetHostInfo 获取宿主机信息
	GetHostInfo() (*domain.HostInfo, error)
}
