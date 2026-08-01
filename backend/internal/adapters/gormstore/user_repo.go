package gormstore

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

// UserRepository 基于 GORM 的用户仓储实现（方言无关）。
type UserRepository struct {
	db *gorm.DB
}

// NewUserRepository 创建用户仓储。
func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// Save 保存用户，按主键冲突时更新字段（原子upsert）。
func (r *UserRepository) Save(user *domain.User) error {
	if user == nil || user.ID == "" {
		return ports.ErrInvalidArgument
	}
	var m userModel
	m.FromDomain(user)
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"username", "password_hash", "email", "role"}),
	}).Create(&m).Error
}

// FindByID 按ID查找用户。
func (r *UserRepository) FindByID(id string) (*domain.User, error) {
	var m userModel
	err := r.db.First(&m, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ports.ErrNotFound
		}
		return nil, err
	}
	return m.ToDomain(), nil
}

// FindByUsername 按用户名查找用户。
func (r *UserRepository) FindByUsername(username string) (*domain.User, error) {
	var m userModel
	err := r.db.First(&m, "username = ?", username).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ports.ErrNotFound
		}
		return nil, err
	}
	return m.ToDomain(), nil
}

// FindAll 返回全部用户，按用户名排序保证确定性。
func (r *UserRepository) FindAll() ([]*domain.User, error) {
	var models []userModel
	if err := r.db.Order("username").Find(&models).Error; err != nil {
		return nil, err
	}
	users := make([]*domain.User, 0, len(models))
	for i := range models {
		users = append(users, models[i].ToDomain())
	}
	return users, nil
}

// Delete 删除用户。
func (r *UserRepository) Delete(id string) error {
	res := r.db.Delete(&userModel{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ports.ErrNotFound
	}
	return nil
}

// 编译期断言：实现 ports 接口
var _ ports.UserRepository = (*UserRepository)(nil)
