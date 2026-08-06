package gormstore

import (
	"crypto/rand"
	"encoding/hex"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"portalt/internal/ports"
)

// newRecordID 生成 32 字符随机十六进制记录ID。
func newRecordID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// vmAccessModel 虚拟机资源授权模型，映射 vm_access 表（004 迁移建表）。
// 记录语义：user_id 被授权访问 vm_id；管理员（vm:manage）不受此表限制。
type vmAccessModel struct {
	ID        string    `gorm:"primaryKey"`
	UserID    string    `gorm:"column:user_id;uniqueIndex:idx_user_vm;not null"`
	VMID      string    `gorm:"column:vm_id;uniqueIndex:idx_user_vm;not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

// TableName 指定表名。
func (vmAccessModel) TableName() string { return "vm_access" }

// VMAccessRepository 基于 GORM 的虚拟机资源授权仓储实现（方言无关）。
type VMAccessRepository struct {
	db *gorm.DB
}

// NewVMAccessRepository 创建虚拟机授权仓储。
func NewVMAccessRepository(db *gorm.DB) *VMAccessRepository {
	return &VMAccessRepository{db: db}
}

// SetForUser 全量替换用户的可见 VM 集合（事务内删除旧记录后批量插入）。
func (r *VMAccessRepository) SetForUser(userID string, vmIDs []string) error {
	if userID == "" {
		return ports.ErrInvalidArgument
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&vmAccessModel{}).Error; err != nil {
			return err
		}
		if len(vmIDs) == 0 {
			return nil
		}
		models := make([]vmAccessModel, 0, len(vmIDs))
		for _, vmID := range vmIDs {
			if vmID == "" {
				continue
			}
			models = append(models, vmAccessModel{
				ID:     newRecordID(),
				UserID: userID,
				VMID:   vmID,
			})
		}
		if len(models) == 0 {
			return nil
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&models).Error
	})
}

// VisibleVMIDs 返回用户全部可见 VM 的 ID 列表。
func (r *VMAccessRepository) VisibleVMIDs(userID string) ([]string, error) {
	if userID == "" {
		return nil, nil
	}
	var rows []string
	if err := r.db.Model(&vmAccessModel{}).
		Where("user_id = ?", userID).
		Order("vm_id").
		Pluck("vm_id", &rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// IsAuthorized 判断用户是否被授权访问指定 VM。
func (r *VMAccessRepository) IsAuthorized(userID, vmID string) (bool, error) {
	if userID == "" || vmID == "" {
		return false, nil
	}
	var count int64
	err := r.db.Model(&vmAccessModel{}).
		Where("user_id = ? AND vm_id = ?", userID, vmID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// DeleteForUser 删除用户的全部授权记录。
func (r *VMAccessRepository) DeleteForUser(userID string) error {
	return r.db.Where("user_id = ?", userID).Delete(&vmAccessModel{}).Error
}

// 编译期断言：实现 ports 接口
var _ ports.VMAccessRepository = (*VMAccessRepository)(nil)
