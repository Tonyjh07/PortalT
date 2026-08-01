// Package domain 是 PortalT 的核心领域模型层。
//
// 遵循盖尔定律设计：本包位于架构最内层，不依赖任何外部框架
// （Gin/GORM等），仅使用标准库，保证业务逻辑可独立验证。
package domain

// VMStatus 虚拟机电源状态类型
type VMStatus string

const (
	// VMStatusPoweredOn 虚拟机已开机
	VMStatusPoweredOn VMStatus = "poweredOn"
	// VMStatusPoweredOff 虚拟机已关机
	VMStatusPoweredOff VMStatus = "poweredOff"
	// VMStatusSuspended 虚拟机已挂起
	VMStatusSuspended VMStatus = "suspended"
	// VMStatusUnknown 虚拟机状态未知（无法确定）
	VMStatusUnknown VMStatus = "unknown"
)

// VM 虚拟机领域实体，描述一台被门户管理的虚拟机。
type VM struct {
	// ID 唯一标识（虚拟化平台侧ID）
	ID string `json:"id"`
	// Name 虚拟机名称
	Name string `json:"name"`
	// Status 电源状态
	Status VMStatus `json:"status"`
	// CPU CPU核数
	CPU int `json:"cpu"`
	// MemoryMB 内存大小（单位MB）
	MemoryMB int `json:"memory_mb"`
	// IPAddress 主IP地址
	IPAddress string `json:"ip_address"`
	// Host 所在宿主机
	Host string `json:"host"`
}

// CanStart 判断虚拟机当前是否允许执行启动操作。
// 仅关机或挂起状态可启动；运行中或未知状态禁止启动，避免误操作。
func (v *VM) CanStart() bool {
	return v.Status == VMStatusPoweredOff || v.Status == VMStatusSuspended
}

// CanStop 判断虚拟机当前是否允许执行停止操作。
// 仅运行中的虚拟机可停止。
func (v *VM) CanStop() bool {
	return v.Status == VMStatusPoweredOn
}

// CanRestart 判断虚拟机当前是否允许执行重启操作。
// 仅运行中的虚拟机可重启。
func (v *VM) CanRestart() bool {
	return v.Status == VMStatusPoweredOn
}

// IsRunning 判断虚拟机是否处于运行状态。
func (v *VM) IsRunning() bool {
	return v.Status == VMStatusPoweredOn
}
