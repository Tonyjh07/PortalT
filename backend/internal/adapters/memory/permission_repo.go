package memory

import (
	"sort"
	"sync"

	"portalt/internal/domain"
)

// PermissionRepository 基于内存的权限字典仓储实现。
type PermissionRepository struct {
	mu    sync.RWMutex
	perms map[string]*domain.PermissionInfo
}

// NewPermissionRepository 创建内存权限字典仓储。
func NewPermissionRepository() *PermissionRepository {
	return &PermissionRepository{perms: make(map[string]*domain.PermissionInfo)}
}

// FindAll 返回全部权限字典条目，按 ID 排序。
func (r *PermissionRepository) FindAll() ([]*domain.PermissionInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*domain.PermissionInfo, 0, len(r.perms))
	for _, p := range r.perms {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Exists 判断权限是否在字典中。
func (r *PermissionRepository) Exists(id string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.perms[id]
	return ok, nil
}

// EnsureDefault 幂等写入默认权限字典（缺失才写入）。
func (r *PermissionRepository) EnsureDefault(perms []domain.PermissionInfo) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range perms {
		if p.ID == "" {
			continue
		}
		if _, ok := r.perms[p.ID]; !ok {
			info := p
			r.perms[p.ID] = &info
		}
	}
	return nil
}
