package postgres

import (
	"gorm.io/gorm"

	"portalt/internal/adapters/gormstore"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

// PluginRepository PostgreSQL 插件仓储。
// 实际逻辑位于共享的 gormstore 包（方言无关），本类型仅做类型绑定。
type PluginRepository struct {
	inner *gormstore.PluginRepository
}

// NewPluginRepository 创建 PostgreSQL 插件仓储。
func NewPluginRepository(db *gorm.DB) *PluginRepository {
	return &PluginRepository{inner: gormstore.NewPluginRepository(db)}
}

// Save 保存插件（原子upsert）。
func (r *PluginRepository) Save(p *domain.Plugin) error { return r.inner.Save(p) }

// FindByID 按ID查找插件。
func (r *PluginRepository) FindByID(id string) (*domain.Plugin, error) { return r.inner.FindByID(id) }

// FindActive 返回全部已启用插件。
func (r *PluginRepository) FindActive() ([]*domain.Plugin, error) { return r.inner.FindActive() }

// FindAll 返回全部插件。
func (r *PluginRepository) FindAll() ([]*domain.Plugin, error) { return r.inner.FindAll() }

// Delete 删除插件。
func (r *PluginRepository) Delete(id string) error { return r.inner.Delete(id) }

// 编译期断言：PostgreSQL 仓储实现 ports 接口
var _ ports.PluginRepository = (*PluginRepository)(nil)
