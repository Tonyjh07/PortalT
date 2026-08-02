package memory

import (
	"sort"
	"sync"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

// RoleRepository 基于内存的角色仓储实现。
type RoleRepository struct {
	mu    sync.RWMutex
	roles map[string]*domain.RoleDefinition
}

// NewRoleRepository 创建内存角色仓储。
func NewRoleRepository() *RoleRepository {
	return &RoleRepository{
		roles: make(map[string]*domain.RoleDefinition),
	}
}

// Save 保存角色，ID 已存在时覆盖。
func (r *RoleRepository) Save(role *domain.RoleDefinition) error {
	if role == nil || role.ID == "" {
		return ports.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.roles[role.ID] = role
	return nil
}

// FindByID 按ID查找角色。
func (r *RoleRepository) FindByID(id string) (*domain.RoleDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	role, ok := r.roles[id]
	if !ok {
		return nil, ports.ErrNotFound
	}
	return role, nil
}

// FindAll 返回全部角色，按 ID 排序（确定性输出）。
func (r *RoleRepository) FindAll() ([]*domain.RoleDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	roles := make([]*domain.RoleDefinition, 0, len(r.roles))
	for _, role := range r.roles {
		roles = append(roles, role)
	}
	sort.Slice(roles, func(i, j int) bool { return roles[i].ID < roles[j].ID })
	return roles, nil
}

// Delete 删除角色。
func (r *RoleRepository) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.roles[id]; !ok {
		return ports.ErrNotFound
	}
	delete(r.roles, id)
	return nil
}
