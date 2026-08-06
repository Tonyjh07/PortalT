package gormstore

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

// permissionModel 权限字典数据库模型，映射 permissions 表（001 迁移建表，本仓储启用）。
type permissionModel struct {
	ID          string    `gorm:"primaryKey"`
	Name        string    `gorm:"uniqueIndex;not null"`
	Description string    `gorm:"not null"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
}

// TableName 指定表名。
func (permissionModel) TableName() string { return "permissions" }

// ToDomain 将数据库模型转换为领域实体。
func (m *permissionModel) ToDomain() *domain.PermissionInfo {
	return &domain.PermissionInfo{ID: m.ID, Description: m.Description}
}

// PermissionRepository 基于 GORM 的权限字典仓储实现（方言无关）。
type PermissionRepository struct {
	db *gorm.DB
}

// NewPermissionRepository 创建权限字典仓储。
func NewPermissionRepository(db *gorm.DB) *PermissionRepository {
	return &PermissionRepository{db: db}
}

// FindAll 返回全部权限字典条目，按 ID 排序。
func (r *PermissionRepository) FindAll() ([]*domain.PermissionInfo, error) {
	var models []permissionModel
	if err := r.db.Order("id").Find(&models).Error; err != nil {
		return nil, err
	}
	out := make([]*domain.PermissionInfo, 0, len(models))
	for i := range models {
		out = append(out, models[i].ToDomain())
	}
	return out, nil
}

// Exists 判断权限是否在字典中。
func (r *PermissionRepository) Exists(id string) (bool, error) {
	var count int64
	err := r.db.Model(&permissionModel{}).Where("id = ?", id).Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// EnsureDefault 幂等写入默认权限字典：缺失才插入，已存在的不覆盖（保留自定义描述）。
func (r *PermissionRepository) EnsureDefault(perms []domain.PermissionInfo) error {
	for _, p := range perms {
		if p.ID == "" {
			continue
		}
		m := permissionModel{ID: p.ID, Name: p.ID, Description: p.Description}
		err := r.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoNothing: true,
		}).Create(&m).Error
		if err != nil {
			return err
		}
	}
	return nil
}

// 编译期断言：实现 ports 接口
var _ ports.PermissionRepository = (*PermissionRepository)(nil)
