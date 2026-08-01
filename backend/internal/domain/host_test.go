package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHostInfo_CPUUsagePercent(t *testing.T) {
	h := &HostInfo{TotalCPU: 8, UsedCPU: 4}
	assert.Equal(t, 50.0, h.CPUUsagePercent())
}

func TestHostInfo_CPUUsagePercent_ZeroTotal(t *testing.T) {
	// 避免除零
	h := &HostInfo{TotalCPU: 0, UsedCPU: 4}
	assert.Equal(t, 0.0, h.CPUUsagePercent())
}

func TestHostInfo_CPUUsagePercent_Nil(t *testing.T) {
	var h *HostInfo
	assert.Equal(t, 0.0, h.CPUUsagePercent())
}

func TestHostInfo_MemoryUsagePercent(t *testing.T) {
	h := &HostInfo{TotalMemoryMB: 16384, UsedMemoryMB: 4096}
	assert.Equal(t, 25.0, h.MemoryUsagePercent())
}

func TestHostInfo_MemoryUsagePercent_ZeroTotal(t *testing.T) {
	h := &HostInfo{TotalMemoryMB: 0, UsedMemoryMB: 100}
	assert.Equal(t, 0.0, h.MemoryUsagePercent())
}

func TestHostInfo_MemoryUsagePercent_Nil(t *testing.T) {
	var h *HostInfo
	assert.Equal(t, 0.0, h.MemoryUsagePercent())
}

func TestHostInfo_JSONFields(t *testing.T) {
	h := &HostInfo{
		Name:          "esxi-01",
		Version:       "8.0",
		CPUModel:      "Xeon",
		TotalCPU:      8,
		UsedCPU:       2,
		TotalMemoryMB: 16384,
		UsedMemoryMB:  4096,
		Status:        "connected",
	}
	json := `{"name":"esxi-01","version":"8.0","cpu_model":"Xeon","total_cpu":8,"used_cpu":2,"total_memory_mb":16384,"used_memory_mb":4096,"status":"connected"}`
	assert.Equal(t, json, string(mustMarshal(t, h)))
}
