package sqlite

import (
	"gorm.io/gorm"

	"portalt/internal/adapters/gormstore"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

// VMRepository SQLite 虚拟机仓储。
// 逻辑位于共享的 gormstore 包，本类型仅做类型绑定。
type VMRepository struct {
	inner *gormstore.VMRepository
}

// NewVMRepository 创建 SQLite 虚拟机仓储。
func NewVMRepository(db *gorm.DB) *VMRepository {
	return &VMRepository{inner: gormstore.NewVMRepository(db)}
}

// Save 保存虚拟机（原子upsert）。
func (r *VMRepository) Save(vm *domain.VM) error { return r.inner.Save(vm) }

// FindByID 按ID查找虚拟机。
func (r *VMRepository) FindByID(id string) (*domain.VM, error) { return r.inner.FindByID(id) }

// FindAll 返回全部虚拟机。
func (r *VMRepository) FindAll() ([]*domain.VM, error) { return r.inner.FindAll() }

// Delete 删除虚拟机。
func (r *VMRepository) Delete(id string) error { return r.inner.Delete(id) }

// 编译期断言：SQLite 仓储实现 ports 接口
var _ ports.VMRepository = (*VMRepository)(nil)
