package gormstore

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

// VMRepository 基于 GORM 的虚拟机仓储实现（方言无关）。
type VMRepository struct {
	db *gorm.DB
}

// NewVMRepository 创建虚拟机仓储。
func NewVMRepository(db *gorm.DB) *VMRepository {
	return &VMRepository{db: db}
}

// Save 保存虚拟机，按主键冲突时更新全部业务字段（原子upsert）。
func (r *VMRepository) Save(vm *domain.VM) error {
	if vm == nil || vm.ID == "" {
		return ports.ErrInvalidArgument
	}
	var m vmModel
	if err := m.FromDomain(vm); err != nil {
		return err
	}
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "status", "cpu", "memory_mb", "ip_address", "host", "metadata"}),
	}).Create(&m).Error
}

// FindByID 按ID查找虚拟机。
func (r *VMRepository) FindByID(id string) (*domain.VM, error) {
	var m vmModel
	err := r.db.First(&m, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ports.ErrNotFound
		}
		return nil, err
	}
	return m.ToDomain()
}

// FindAll 返回全部虚拟机，按名称排序保证确定性。
func (r *VMRepository) FindAll() ([]*domain.VM, error) {
	var models []vmModel
	if err := r.db.Order("name").Find(&models).Error; err != nil {
		return nil, err
	}
	vms := make([]*domain.VM, 0, len(models))
	for i := range models {
		vm, err := models[i].ToDomain()
		if err != nil {
			return nil, err
		}
		vms = append(vms, vm)
	}
	return vms, nil
}

// Delete 删除虚拟机。
func (r *VMRepository) Delete(id string) error {
	res := r.db.Delete(&vmModel{}, "id = ?", id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ports.ErrNotFound
	}
	return nil
}

// 编译期断言：实现 ports 接口
var _ ports.VMRepository = (*VMRepository)(nil)
