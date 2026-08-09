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

// pluginModel 插件数据库模型，映射 plugins 表。
// endpoints / manifest_json 以 JSON 文本存储（SQLite TEXT / PostgreSQL TEXT 均可）。
type pluginModel struct {
	ID           string    `gorm:"primaryKey"`
	Name         string    `gorm:"not null"`
	Icon         string    `gorm:"not null"`
	Route        string    `gorm:"uniqueIndex;not null"`
	Type         string    `gorm:"not null;default:access"`
	IframeURL    string    `gorm:"column:iframe_url;not null"`
	ApiURL       string    `gorm:"column:api_url;not null"`
	Endpoints    string    `gorm:"not null"`
	CaddyRules   string    `gorm:"column:caddy_rules;not null"`
	Permission   string    `gorm:"not null"`
	SortOrder    int       `gorm:"column:sort_order;not null"`
	IsActive     bool      `gorm:"column:is_active;not null"`
	Status       string    `gorm:"not null"`
	ManifestJSON string    `gorm:"column:manifest_json;not null"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime"`
}

// TableName 指定表名。
func (pluginModel) TableName() string { return "plugins" }

// ToDomain 将数据库模型转换为领域实体。
func (m *pluginModel) ToDomain() (*domain.Plugin, error) {
	p := &domain.Plugin{
		ID:           m.ID,
		Name:         m.Name,
		Icon:         m.Icon,
		Route:        m.Route,
		Type:         domain.NormalizePluginType(domain.PluginType(m.Type)),
		IframeURL:    m.IframeURL,
		ApiURL:       m.ApiURL,
		CaddyRules:   m.CaddyRules,
		Permission:   m.Permission,
		SortOrder:    m.SortOrder,
		IsActive:     m.IsActive,
		Status:       m.Status,
		ManifestJSON: m.ManifestJSON,
	}
	if m.Endpoints != "" {
		if err := json.Unmarshal([]byte(m.Endpoints), &p.Endpoints); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// FromDomain 将领域实体写入数据库模型。
func (m *pluginModel) FromDomain(p *domain.Plugin) error {
	b, err := json.Marshal(p.Endpoints)
	if err != nil {
		return err
	}
	m.ID = p.ID
	m.Name = p.Name
	m.Icon = p.Icon
	m.Route = p.Route
	m.Type = string(domain.NormalizePluginType(p.Type))
	m.IframeURL = p.IframeURL
	m.ApiURL = p.ApiURL
	m.Endpoints = string(b)
	m.CaddyRules = p.CaddyRules
	m.Permission = p.Permission
	m.SortOrder = p.SortOrder
	m.IsActive = p.IsActive
	m.Status = p.Status
	m.ManifestJSON = p.ManifestJSON
	return nil
}

// PluginRepository 基于 GORM 的插件仓储实现（方言无关）。
type PluginRepository struct {
	db *gorm.DB
}

// NewPluginRepository 创建插件仓储。
func NewPluginRepository(db *gorm.DB) *PluginRepository {
	return &PluginRepository{db: db}
}

// Save 保存插件，按主键冲突时更新全部业务字段（原子upsert）。
func (r *PluginRepository) Save(p *domain.Plugin) error {
	if p == nil || p.ID == "" {
		return ports.ErrInvalidArgument
	}
	var m pluginModel
	m.FromDomain(p)
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "icon", "route", "type", "iframe_url", "api_url", "endpoints", "caddy_rules", "permission", "sort_order", "is_active", "status", "manifest_json", "updated_at"}),
	}).Create(&m).Error
}

// FindByID 按ID查找插件。
func (r *PluginRepository) FindByID(id string) (*domain.Plugin, error) {
	var m pluginModel
	err := r.db.First(&m, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ports.ErrNotFound
		}
		return nil, err
	}
	return m.ToDomain()
}

// FindActive 返回全部已启用插件，按 sort_order 升序。
func (r *PluginRepository) FindActive() ([]*domain.Plugin, error) {
	return r.find("is_active = ?", true)
}

// FindAll 返回全部插件（含停用），按 sort_order 升序。
func (r *PluginRepository) FindAll() ([]*domain.Plugin, error) {
	return r.find("", nil)
}

func (r *PluginRepository) find(cond string, args ...any) ([]*domain.Plugin, error) {
	q := r.db.Order("sort_order").Order("name")
	if cond != "" {
		q = q.Where(cond, args...)
	}
	var models []pluginModel
	if err := q.Find(&models).Error; err != nil {
		return nil, err
	}
	plugins := make([]*domain.Plugin, 0, len(models))
	for i := range models {
		p, err := models[i].ToDomain()
		if err != nil {
			return nil, err
		}
		plugins = append(plugins, p)
	}
	return plugins, nil
}

// Delete 删除插件。
func (r *PluginRepository) Delete(id string) error {
	res := r.db.Delete(&pluginModel{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ports.ErrNotFound
	}
	return nil
}

// 编译期断言：实现 ports 接口
var _ ports.PluginRepository = (*PluginRepository)(nil)
