// Package memory 提供基于内存的仓储实现（测试与开发环境用）。
//
// 使用 sync.RWMutex + map 保证并发安全，
// 与 PostgreSQL 实现共享同一 ports 接口，验证可替换性。
package memory

import (
	"sync"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

// VMRepository 基于内存的虚拟机仓储实现。
type VMRepository struct {
	mu  sync.RWMutex
	vms map[string]*domain.VM
}

// NewVMRepository 创建内存虚拟机仓储。
func NewVMRepository() *VMRepository {
	return &VMRepository{
		vms: make(map[string]*domain.VM),
	}
}

// Save 保存虚拟机，ID 已存在时覆盖（upsert语义）。
func (r *VMRepository) Save(vm *domain.VM) error {
	if vm == nil || vm.ID == "" {
		return ports.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.vms[vm.ID] = vm
	return nil
}

// FindByID 按ID查找虚拟机。
func (r *VMRepository) FindByID(id string) (*domain.VM, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	vm, ok := r.vms[id]
	if !ok {
		return nil, ports.ErrNotFound
	}
	return vm, nil
}

// FindAll 返回全部虚拟机，无数据时返回空切片。
func (r *VMRepository) FindAll() ([]*domain.VM, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	vms := make([]*domain.VM, 0, len(r.vms))
	for _, vm := range r.vms {
		vms = append(vms, vm)
	}
	return vms, nil
}

// Delete 删除虚拟机。
func (r *VMRepository) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.vms[id]; !ok {
		return ports.ErrNotFound
	}
	delete(r.vms, id)
	return nil
}
