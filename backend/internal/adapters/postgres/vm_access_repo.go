package postgres

import (
	"gorm.io/gorm"

	"portalt/internal/adapters/gormstore"
	"portalt/internal/ports"
)

// VMAccessRepository PostgreSQL 虚拟机授权仓储。
// 实际逻辑位于共享的 gormstore 包（方言无关），本类型仅做类型绑定。
type VMAccessRepository struct {
	inner *gormstore.VMAccessRepository
}

// NewVMAccessRepository 创建 PostgreSQL 虚拟机授权仓储。
func NewVMAccessRepository(db *gorm.DB) *VMAccessRepository {
	return &VMAccessRepository{inner: gormstore.NewVMAccessRepository(db)}
}

// SetForUser 全量替换用户的可见 VM 集合。
func (r *VMAccessRepository) SetForUser(userID string, vmIDs []string) error {
	return r.inner.SetForUser(userID, vmIDs)
}

// VisibleVMIDs 返回用户全部可见 VM 的 ID 列表。
func (r *VMAccessRepository) VisibleVMIDs(userID string) ([]string, error) {
	return r.inner.VisibleVMIDs(userID)
}

// IsAuthorized 判断用户是否被授权访问指定 VM。
func (r *VMAccessRepository) IsAuthorized(userID, vmID string) (bool, error) {
	return r.inner.IsAuthorized(userID, vmID)
}

// DeleteForUser 删除用户的全部授权记录。
func (r *VMAccessRepository) DeleteForUser(userID string) error { return r.inner.DeleteForUser(userID) }

// 编译期断言：实现 ports 接口
var _ ports.VMAccessRepository = (*VMAccessRepository)(nil)
