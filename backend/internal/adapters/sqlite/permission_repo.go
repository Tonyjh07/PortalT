package sqlite

import (
	"gorm.io/gorm"

	"portalt/internal/adapters/gormstore"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

// PermissionRepository SQLite 权限字典仓储。
// 实际逻辑位于共享的 gormstore 包（方言无关），本类型仅做类型绑定。
type PermissionRepository struct {
	inner *gormstore.PermissionRepository
}

// NewPermissionRepository 创建 SQLite 权限字典仓储。
func NewPermissionRepository(db *gorm.DB) *PermissionRepository {
	return &PermissionRepository{inner: gormstore.NewPermissionRepository(db)}
}

// FindAll 返回全部权限字典条目。
func (r *PermissionRepository) FindAll() ([]*domain.PermissionInfo, error) { return r.inner.FindAll() }

// Exists 判断权限是否在字典中。
func (r *PermissionRepository) Exists(id string) (bool, error) { return r.inner.Exists(id) }

// EnsureDefault 幂等写入默认权限字典。
func (r *PermissionRepository) EnsureDefault(perms []domain.PermissionInfo) error {
	return r.inner.EnsureDefault(perms)
}

// 编译期断言：实现 ports 接口
var _ ports.PermissionRepository = (*PermissionRepository)(nil)
