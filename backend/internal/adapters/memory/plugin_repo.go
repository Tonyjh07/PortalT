package memory

import (
	"sort"
	"sync"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

// PluginRepository 基于内存的插件仓储实现。
type PluginRepository struct {
	mu      sync.RWMutex
	plugins map[string]*domain.Plugin
}

// NewPluginRepository 创建内存插件仓储。
func NewPluginRepository() *PluginRepository {
	return &PluginRepository{
		plugins: make(map[string]*domain.Plugin),
	}
}

// Save 保存插件，ID 已存在时覆盖（upsert语义）。
func (r *PluginRepository) Save(p *domain.Plugin) error {
	if p == nil || p.ID == "" {
		return ports.ErrInvalidArgument
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins[p.ID] = p
	return nil
}

// FindByID 按ID查找插件。
func (r *PluginRepository) FindByID(id string) (*domain.Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[id]
	if !ok {
		return nil, ports.ErrNotFound
	}
	return p, nil
}

// FindActive 返回全部已启用插件，按 SortOrder 升序。
func (r *PluginRepository) FindActive() ([]*domain.Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*domain.Plugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		if p.IsActive {
			out = append(out, p)
		}
	}
	sortPlugins(out)
	return out, nil
}

// FindAll 返回全部插件（含停用），按 SortOrder 升序。
func (r *PluginRepository) FindAll() ([]*domain.Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*domain.Plugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		out = append(out, p)
	}
	sortPlugins(out)
	return out, nil
}

// Delete 删除插件。
func (r *PluginRepository) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.plugins[id]; !ok {
		return ports.ErrNotFound
	}
	delete(r.plugins, id)
	return nil
}

// sortPlugins 按 SortOrder 升序、同值按 Name 排序（确定性）。
func sortPlugins(ps []*domain.Plugin) {
	sort.Slice(ps, func(i, j int) bool {
		if ps[i].SortOrder != ps[j].SortOrder {
			return ps[i].SortOrder < ps[j].SortOrder
		}
		return ps[i].Name < ps[j].Name
	})
}

// 编译期断言：实现 ports 接口
var _ ports.PluginRepository = (*PluginRepository)(nil)
