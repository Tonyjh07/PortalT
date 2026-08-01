// Package ports 定义架构中间层接口（依赖倒置）。
//
// 本层只定义接口与错误哨兵，不包含任何实现，
// 保证内层领域逻辑与具体存储/虚拟化实现解耦。
package ports

import (
	"errors"

	"portalt/internal/domain"
)

// 仓储层通用错误哨兵。
var (
	// ErrNotFound 记录不存在
	ErrNotFound = errors.New("record not found")
	// ErrInvalidArgument 参数无效（如空ID、nil对象）
	ErrInvalidArgument = errors.New("invalid argument")
	// ErrInvalidOperation 业务操作在当前状态不允许（如对运行中的VM执行启动）
	ErrInvalidOperation = errors.New("invalid operation")
)

// VMRepository 虚拟机数据仓储接口。
// 由 adapters 层实现（memory/postgres），供领域服务使用。
type VMRepository interface {
	// Save 保存虚拟机（存在则覆盖，upsert语义）
	Save(vm *domain.VM) error
	// FindByID 按ID查找虚拟机，不存在返回 ErrNotFound
	FindByID(id string) (*domain.VM, error)
	// FindAll 返回全部虚拟机
	FindAll() ([]*domain.VM, error)
	// Delete 按ID删除虚拟机，不存在返回 ErrNotFound
	Delete(id string) error
}

// UserRepository 用户数据仓储接口。
// 由 adapters 层实现，供认证与用户管理使用。
type UserRepository interface {
	// Save 保存用户（存在则覆盖）
	Save(user *domain.User) error
	// FindByID 按ID查找用户，不存在返回 ErrNotFound
	FindByID(id string) (*domain.User, error)
	// FindByUsername 按用户名查找用户，不存在返回 ErrNotFound
	FindByUsername(username string) (*domain.User, error)
	// FindAll 返回全部用户
	FindAll() ([]*domain.User, error)
	// Delete 按ID删除用户，不存在返回 ErrNotFound
	Delete(id string) error
}

// PluginRepository 插件（动态菜单）数据仓储接口。
// 由 adapters 层实现，供菜单与插件管理使用。
type PluginRepository interface {
	// Save 保存插件（存在则覆盖，upsert语义）
	Save(plugin *domain.Plugin) error
	// FindByID 按ID查找插件，不存在返回 ErrNotFound
	FindByID(id string) (*domain.Plugin, error)
	// FindActive 返回全部已启用插件，按 SortOrder 升序（确定性排序）
	FindActive() ([]*domain.Plugin, error)
	// FindAll 返回全部插件（含停用），按 SortOrder 升序
	FindAll() ([]*domain.Plugin, error)
	// Delete 按ID删除插件，不存在返回 ErrNotFound
	Delete(id string) error
}
