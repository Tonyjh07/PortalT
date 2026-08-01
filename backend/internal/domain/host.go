package domain

// HostInfo 虚拟化平台宿主机信息实体。
// 由虚拟化提供者填充，供仪表盘展示资源使用情况。
type HostInfo struct {
	// Name 宿主机名称
	Name string `json:"name"`
	// Version 虚拟化平台版本
	Version string `json:"version"`
	// CPUModel CPU型号描述
	CPUModel string `json:"cpu_model"`
	// TotalCPU CPU总核数
	TotalCPU int `json:"total_cpu"`
	// UsedCPU 已使用CPU核数
	UsedCPU int `json:"used_cpu"`
	// TotalMemoryMB 内存总量（MB）
	TotalMemoryMB int `json:"total_memory_mb"`
	// UsedMemoryMB 已使用内存（MB）
	UsedMemoryMB int `json:"used_memory_mb"`
	// Status 平台连接状态（connected / disconnected）
	Status string `json:"status"`
}

// CPUUsagePercent 计算CPU使用率百分比，避免除零。
func (h *HostInfo) CPUUsagePercent() float64 {
	if h == nil || h.TotalCPU <= 0 {
		return 0
	}
	return float64(h.UsedCPU) / float64(h.TotalCPU) * 100
}

// MemoryUsagePercent 计算内存使用率百分比，避免除零。
func (h *HostInfo) MemoryUsagePercent() float64 {
	if h == nil || h.TotalMemoryMB <= 0 {
		return 0
	}
	return float64(h.UsedMemoryMB) / float64(h.TotalMemoryMB) * 100
}
