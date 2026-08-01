// Package gormstore 提供方言无关的 GORM 仓储实现。
//
// postgres 与 sqlite 适配器共用本包的数据库模型与仓储逻辑，
// 各自只负责连接打开（Open）与方言迁移脚本（Migrate）。
package gormstore

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"portalt/internal/domain"
)

// vmModel 虚拟机数据库模型，映射 vms 表。
// jsonb 类型在 SQLite 中自动降级为 TEXT 亲和类型，无兼容问题。
type vmModel struct {
	ID        string    `gorm:"primaryKey"`
	Name      string    `gorm:"not null"`
	Status    string    `gorm:"not null"`
	CPU       int       `gorm:"not null"`
	MemoryMB  int       `gorm:"column:memory_mb;not null"`
	IPAddress string    `gorm:"column:ip_address;not null"`
	Host      string    `gorm:"not null"`
	Metadata  []byte    `gorm:"type:jsonb;not null"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// TableName 指定表名。
func (vmModel) TableName() string { return "vms" }

// ToDomain 将数据库模型转换为领域实体。
func (m *vmModel) ToDomain() (*domain.VM, error) {
	vm := &domain.VM{
		ID:        m.ID,
		Name:      m.Name,
		Status:    domain.VMStatus(m.Status),
		CPU:       m.CPU,
		MemoryMB:  m.MemoryMB,
		IPAddress: m.IPAddress,
		Host:      m.Host,
	}
	if len(m.Metadata) > 0 {
		// 空对象/空值视为未设置，保持 nil 语义与内存仓储一致
		s := strings.TrimSpace(string(m.Metadata))
		if s != "{}" && s != "null" {
			if err := json.Unmarshal(m.Metadata, &vm.Metadata); err != nil {
				return nil, fmt.Errorf("parse metadata: %w", err)
			}
		}
	}
	return vm, nil
}

// FromDomain 将领域实体写入数据库模型。
func (m *vmModel) FromDomain(vm *domain.VM) error {
	m.ID = vm.ID
	m.Name = vm.Name
	m.Status = string(vm.Status)
	m.CPU = vm.CPU
	m.MemoryMB = vm.MemoryMB
	m.IPAddress = vm.IPAddress
	m.Host = vm.Host
	if vm.Metadata == nil {
		m.Metadata = []byte("{}")
		return nil
	}
	b, err := json.Marshal(vm.Metadata)
	if err != nil {
		return fmt.Errorf("serialize metadata: %w", err)
	}
	m.Metadata = b
	return nil
}

// userModel 用户数据库模型，映射 users 表。
type userModel struct {
	ID           string    `gorm:"primaryKey"`
	Username     string    `gorm:"uniqueIndex;not null"`
	PasswordHash string    `gorm:"column:password_hash;not null"`
	Email        string    `gorm:"not null"`
	Role         string    `gorm:"not null"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
}

// TableName 指定表名。
func (userModel) TableName() string { return "users" }

// ToDomain 将数据库模型转换为领域实体。
func (m *userModel) ToDomain() *domain.User {
	return &domain.User{
		ID:       m.ID,
		Username: m.Username,
		Password: m.PasswordHash,
		Email:    m.Email,
		Role:     domain.Role(m.Role),
	}
}

// FromDomain 将领域实体写入数据库模型。
func (m *userModel) FromDomain(user *domain.User) {
	m.ID = user.ID
	m.Username = user.Username
	m.PasswordHash = user.Password
	m.Email = user.Email
	m.Role = string(user.Role)
}
