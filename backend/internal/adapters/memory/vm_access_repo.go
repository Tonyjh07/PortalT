package memory

import (
	"sort"
	"sync"

	"portalt/internal/ports"
)

// VMAccessRepository 基于内存的虚拟机资源授权仓储实现。
type VMAccessRepository struct {
	mu    sync.RWMutex
	byUser map[string]map[string]struct{}
}

// NewVMAccessRepository 创建内存虚拟机授权仓储。
func NewVMAccessRepository() *VMAccessRepository {
	return &VMAccessRepository{byUser: make(map[string]map[string]struct{})}
}

// SetForUser 全量替换用户的可见 VM 集合。
func (r *VMAccessRepository) SetForUser(userID string, vmIDs []string) error {
	if userID == "" {
		return ports.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	set := make(map[string]struct{}, len(vmIDs))
	for _, id := range vmIDs {
		if id != "" {
			set[id] = struct{}{}
		}
	}
	r.byUser[userID] = set
	return nil
}

// VisibleVMIDs 返回用户全部可见 VM 的 ID 列表（排序输出，确定性）。
func (r *VMAccessRepository) VisibleVMIDs(userID string) ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	set, ok := r.byUser[userID]
	if !ok {
		return nil, nil
	}
	out := make([]string, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

// IsAuthorized 判断用户是否被授权访问指定 VM。
func (r *VMAccessRepository) IsAuthorized(userID, vmID string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	set, ok := r.byUser[userID]
	if !ok {
		return false, nil
	}
	_, ok = set[vmID]
	return ok, nil
}

// DeleteForUser 删除用户的全部授权记录。
func (r *VMAccessRepository) DeleteForUser(userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.byUser, userID)
	return nil
}
