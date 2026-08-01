package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// vmWithStatus 构造指定状态的VM
func vmWithStatus(status VMStatus) *VM {
	return &VM{
		ID:        "vm-1",
		Name:      "test-vm",
		Status:    status,
		CPU:       2,
		MemoryMB:  4096,
		IPAddress: "192.168.1.10",
		Host:      "esxi-01",
	}
}

func TestVM_CanStart(t *testing.T) {
	tests := []struct {
		name   string
		status VMStatus
		want   bool
	}{
		{"关机可启动", VMStatusPoweredOff, true},
		{"挂起可启动", VMStatusSuspended, true},
		{"运行中不可启动", VMStatusPoweredOn, false},
		{"未知状态不可启动", VMStatusUnknown, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := vmWithStatus(tt.status)
			assert.Equal(t, tt.want, vm.CanStart())
		})
	}
}

func TestVM_CanStop(t *testing.T) {
	tests := []struct {
		name   string
		status VMStatus
		want   bool
	}{
		{"运行中可停止", VMStatusPoweredOn, true},
		{"已关机不可停止", VMStatusPoweredOff, false},
		{"挂起不可停止", VMStatusSuspended, false},
		{"未知状态不可停止", VMStatusUnknown, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := vmWithStatus(tt.status)
			assert.Equal(t, tt.want, vm.CanStop())
		})
	}
}

func TestVM_CanRestart(t *testing.T) {
	tests := []struct {
		name   string
		status VMStatus
		want   bool
	}{
		{"运行中可重启", VMStatusPoweredOn, true},
		{"已关机不可重启", VMStatusPoweredOff, false},
		{"挂起不可重启", VMStatusSuspended, false},
		{"未知状态不可重启", VMStatusUnknown, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := vmWithStatus(tt.status)
			assert.Equal(t, tt.want, vm.CanRestart())
		})
	}
}

func TestVM_IsRunning(t *testing.T) {
	tests := []struct {
		name   string
		status VMStatus
		want   bool
	}{
		{"运行中", VMStatusPoweredOn, true},
		{"已关机", VMStatusPoweredOff, false},
		{"挂起", VMStatusSuspended, false},
		{"未知", VMStatusUnknown, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vm := vmWithStatus(tt.status)
			assert.Equal(t, tt.want, vm.IsRunning())
		})
	}
}

func TestVM_JSONFields(t *testing.T) {
	// 验证JSON序列化使用预期字段名（API契约）
	vm := vmWithStatus(VMStatusPoweredOn)
	json := `{"id":"vm-1","name":"test-vm","status":"poweredOn","cpu":2,"memory_mb":4096,"ip_address":"192.168.1.10","host":"esxi-01","metadata":null}`
	assert.Equal(t, json, string(mustMarshal(t, vm)))
}

func TestVM_MetadataJSON(t *testing.T) {
	vm := vmWithStatus(VMStatusPoweredOn)
	vm.Metadata = map[string]any{"proto": "rdp", "port": 3389}

	json := `{"id":"vm-1","name":"test-vm","status":"poweredOn","cpu":2,"memory_mb":4096,"ip_address":"192.168.1.10","host":"esxi-01","metadata":{"port":3389,"proto":"rdp"}}`
	assert.Equal(t, json, string(mustMarshal(t, vm)))
}
