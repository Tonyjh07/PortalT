// Package services 实现领域业务服务。
//
// 服务层编排仓储与外部提供者（虚拟化平台等），
// 只依赖 ports 层接口，不感知具体实现。
package services

import (
	"context"
	"fmt"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

// VMService 虚拟机业务服务，处理VM同步与操作编排。
type VMService struct {
	repo     ports.VMRepository
	provider ports.VirtualizationProvider
}

// NewVMService 创建VM服务，注入仓储与虚拟化提供者。
func NewVMService(repo ports.VMRepository, provider ports.VirtualizationProvider) *VMService {
	return &VMService{
		repo:     repo,
		provider: provider,
	}
}

// SyncVMs 从虚拟化提供者同步全部虚拟机到仓储。
//
// 同步语义：保存提供者返回的全部VM，并删除提供者中已不存在的
// 陈旧记录。提供者返回错误时立即中止，不做任何变更（安全兜底）。
// 返回本次同步的VM数量。
func (s *VMService) SyncVMs(ctx context.Context) (int, error) {
	vms, err := s.provider.ListVMs()
	if err != nil {
		return 0, fmt.Errorf("list vms from provider: %w", err)
	}

	seen := make(map[string]struct{}, len(vms))
	for _, vm := range vms {
		if vm == nil || vm.ID == "" {
			continue
		}
		seen[vm.ID] = struct{}{}
		if err := s.repo.Save(vm); err != nil {
			return 0, fmt.Errorf("save vm %q: %w", vm.ID, err)
		}
	}

	existing, err := s.repo.FindAll()
	if err != nil {
		return 0, fmt.Errorf("find existing vms: %w", err)
	}
	for _, vm := range existing {
		if _, ok := seen[vm.ID]; !ok {
			if err := s.repo.Delete(vm.ID); err != nil {
				return 0, fmt.Errorf("delete stale vm %q: %w", vm.ID, err)
			}
		}
	}

	return len(vms), nil
}

// ListVMs 返回仓储中的全部虚拟机。
func (s *VMService) ListVMs(ctx context.Context) ([]*domain.VM, error) {
	return s.repo.FindAll()
}

// GetVM 按ID返回虚拟机详情。
func (s *VMService) GetVM(ctx context.Context, id string) (*domain.VM, error) {
	return s.repo.FindByID(id)
}

// StartVM 启动虚拟机。
// 流程：加载 → 校验状态规则（仅关机/挂起可启动）→ 调用提供者 → 回刷状态并保存。
func (s *VMService) StartVM(ctx context.Context, id string) (*domain.VM, error) {
	return s.powerOp(ctx, id, func(vm *domain.VM) error {
		if !vm.CanStart() {
			return fmt.Errorf("vm %q: %w: 当前状态 %s 不允许启动", id, ports.ErrInvalidOperation, vm.Status)
		}
		return s.provider.StartVM(id)
	})
}

// StopVM 停止虚拟机（仅运行中可停止）。
func (s *VMService) StopVM(ctx context.Context, id string) (*domain.VM, error) {
	return s.powerOp(ctx, id, func(vm *domain.VM) error {
		if !vm.CanStop() {
			return fmt.Errorf("vm %q: %w: 当前状态 %s 不允许停止", id, ports.ErrInvalidOperation, vm.Status)
		}
		return s.provider.StopVM(id)
	})
}

// RestartVM 重启虚拟机（仅运行中可重启）。
func (s *VMService) RestartVM(ctx context.Context, id string) (*domain.VM, error) {
	return s.powerOp(ctx, id, func(vm *domain.VM) error {
		if !vm.CanRestart() {
			return fmt.Errorf("vm %q: %w: 当前状态 %s 不允许重启", id, ports.ErrInvalidOperation, vm.Status)
		}
		return s.provider.RestartVM(id)
	})
}

// GetVMStatus 获取虚拟机实时状态（轮询用）。
// 优先从提供者查询最新状态并回写仓储；提供者不可达时回退到仓储缓存。
func (s *VMService) GetVMStatus(ctx context.Context, id string) (*domain.VM, error) {
	live, err := s.provider.ListVMs()
	if err == nil {
		for _, vm := range live {
			if vm != nil && vm.ID == id {
				if err := s.repo.Save(vm); err != nil {
					return nil, fmt.Errorf("refresh vm %q: %w", id, err)
				}
				return vm, nil
			}
		}
	}
	return s.repo.FindByID(id)
}

// powerOp 电源操作公共编排：加载 → 校验规则 → 执行 → 回刷状态。
func (s *VMService) powerOp(ctx context.Context, id string, op func(*domain.VM) error) (*domain.VM, error) {
	vm, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if err := op(vm); err != nil {
		return nil, err
	}
	// 操作成功后从提供者回刷最新状态（VM被误删等极端情况以提供者为准）
	if refreshed, err := s.GetVMStatus(ctx, id); err == nil {
		return refreshed, nil
	}
	return vm, nil
}
