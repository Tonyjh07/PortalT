package gormstore

import (
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

// roleModel 角色数据库模型，映射 roles 表。
// permissions 以 JSON 数组文本存储（SQLite TEXT / PostgreSQL TEXT 均可）。
type roleModel struct {
	ID          string    `gorm:"primaryKey"`
	Name        string    `gorm:"not null"`
	Description string    `gorm:"not null"`
	Permissions string    `gorm:"not null"`
	CreatedAt   time.Time `gorm:"autoCreateTime"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime"`
}

// TableName 指定表名。
func (roleModel) TableName() string { return "roles" }

// ToDomain 将数据库模型转换为领域实体。
func (m *roleModel) ToDomain() (*domain.RoleDefinition, error) {
	perms := []string{}
	if m.Permissions != "" {
		if err := json.Unmarshal([]byte(m.Permissions), &perms); err != nil {
			return nil, err
		}
	}
	return &domain.RoleDefinition{
		ID:          m.ID,
		Name:        m.Name,
		Description: m.Description,
		Permissions: perms,
	}, nil
}

// FromDomain 将领域实体写入数据库模型。
func (m *roleModel) FromDomain(r *domain.RoleDefinition) error {
	b, err := json.Marshal(r.Permissions)
	if err != nil {
		return err
	}
	m.ID = r.ID
	m.Name = r.Name
	m.Description = r.Description
	m.Permissions = string(b)
	return nil
}

// RoleRepository 基于 GORM 的角色仓储实现（方言无关）。
type RoleRepository struct {
	db *gorm.DB
}

// NewRoleRepository 创建角色仓储。
func NewRoleRepository(db *gorm.DB) *RoleRepository {
	return &RoleRepository{db: db}
}

// Save 保存角色，按主键冲突时更新全部业务字段（原子upsert）。
func (r *RoleRepository) Save(role *domain.RoleDefinition) error {
	if role == nil || role.ID == "" {
		return ports.ErrInvalidArgument
	}
	var m roleModel
	if err := m.FromDomain(role); err != nil {
		return err
	}
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "description", "permissions"}),
	}).Create(&m).Error
}

// FindByID 按ID查找角色。
func (r *RoleRepository) FindByID(id string) (*domain.RoleDefinition, error) {
	var m roleModel
	err := r.db.First(&m, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ports.ErrNotFound
		}
		return nil, err
	}
	return m.ToDomain()
}

// FindAll 返回全部角色，按 ID 排序。
func (r *RoleRepository) FindAll() ([]*domain.RoleDefinition, error) {
	var models []roleModel
	if err := r.db.Order("id").Find(&models).Error; err != nil {
		return nil, err
	}
	roles := make([]*domain.RoleDefinition, 0, len(models))
	for i := range models {
		r, err := models[i].ToDomain()
		if err != nil {
			return nil, err
		}
		roles = append(roles, r)
	}
	return roles, nil
}

// Delete 删除角色。
func (r *RoleRepository) Delete(id string) error {
	res := r.db.Delete(&roleModel{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ports.ErrNotFound
	}
	return nil
}

// 编译期断言：实现 ports 接口
var _ ports.RoleRepository = (*RoleRepository)(nil)
