//go:build integration

// 真实 ESXi 电源操作集成测试。
//
// 前置：ESXi 上至少有一台测试虚拟机（TEST_ESXI_VM 指定名称，缺省取列表中第一台）。
// 注意：本测试会真实关机/开机/重启该虚拟机，勿指向生产 VM。
package esxi

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"portalt/internal/domain"
)

// pickTargetVM 返回电源操作目标 VM 的 ID：优先 TEST_ESXI_VM 指定名称，
// 否则取列表中第一台处于 poweredOn 的 VM（保证 StopVM 不会因
// InvalidPowerState 失败），都没有才退而取第一台。
func pickTargetVM(t *testing.T, p *Provider) string {
	t.Helper()
	vms, err := p.ListVMs()
	require.NoError(t, err)
	require.NotEmpty(t, vms, "ESXi 上没有虚拟机，无法测试电源操作")

	if name := os.Getenv("TEST_ESXI_VM"); name != "" {
		for _, vm := range vms {
			if vm.Name == name {
				t.Logf("目标 VM（按名称）: %s (%s)", vm.Name, vm.ID)
				return vm.ID
			}
		}
		t.Fatalf("未找到名为 %s 的虚拟机", name)
	}
	for _, vm := range vms {
		if vm.Status == domain.VMStatusPoweredOn {
			t.Logf("目标 VM（取第一台运行中）: %s (%s)", vm.Name, vm.ID)
			return vm.ID
		}
	}
	t.Logf("目标 VM（取第一台）: %s (%s)", vms[0].Name, vms[0].ID)
	return vms[0].ID
}

func waitStatus(t *testing.T, p *Provider, id string, want domain.VMStatus) domain.VMStatus {
	t.Helper()
	var got domain.VMStatus
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		vms, err := p.ListVMs()
		if err != nil {
			t.Logf("轮询状态出错（重试）: %v", err)
		} else {
			for _, vm := range vms {
				if vm.ID == id {
					got = vm.Status
					if got == want {
						return got
					}
				}
			}
		}
		time.Sleep(3 * time.Second)
	}
	return got
}

// TestRealESXi_PowerCycle 完整验证 关/开/重启 三个电源操作与状态机流转。
func TestRealESXi_PowerCycle(t *testing.T) {
	p := newRealProvider(t)
	id := pickTargetVM(t, p)

	// 1. 关机
	require.NoError(t, p.StopVM(id))
	got := waitStatus(t, p, id, domain.VMStatusPoweredOff)
	require.Equal(t, domain.VMStatusPoweredOff, got, "关机后状态应为 poweredOff")

	// 2. 开机
	require.NoError(t, p.StartVM(id))
	got = waitStatus(t, p, id, domain.VMStatusPoweredOn)
	require.Equal(t, domain.VMStatusPoweredOn, got, "开机后状态应为 poweredOn")

	// 3. 重启
	require.NoError(t, p.RestartVM(id))
	got = waitStatus(t, p, id, domain.VMStatusPoweredOn)
	require.Equal(t, domain.VMStatusPoweredOn, got, "重启后状态应为 poweredOn")
	t.Logf("电源操作全链路 OK，最终状态: %s", got)
}
