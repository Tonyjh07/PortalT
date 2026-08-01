package postgres

import (
	"gorm.io/gorm"

	"portalt/internal/adapters/gormstore"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

// UserRepository PostgreSQL 用户仓储。
// 实际逻辑位于共享的 gormstore 包（方言无关），本类型仅做类型绑定。
type UserRepository struct {
	inner *gormstore.UserRepository
}

// NewUserRepository 创建 PostgreSQL 用户仓储。
func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{inner: gormstore.NewUserRepository(db)}
}

// Save 保存用户（原子upsert）。
func (r *UserRepository) Save(user *domain.User) error { return r.inner.Save(user) }

// FindByID 按ID查找用户。
func (r *UserRepository) FindByID(id string) (*domain.User, error) { return r.inner.FindByID(id) }

// FindByUsername 按用户名查找用户。
func (r *UserRepository) FindByUsername(username string) (*domain.User, error) {
	return r.inner.FindByUsername(username)
}

// FindAll 返回全部用户。
func (r *UserRepository) FindAll() ([]*domain.User, error) { return r.inner.FindAll() }

// Delete 删除用户。
func (r *UserRepository) Delete(id string) error { return r.inner.Delete(id) }

// 编译期断言：PostgreSQL 仓储实现 ports 接口
var _ ports.UserRepository = (*UserRepository)(nil)
