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
