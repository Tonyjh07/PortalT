// Package ports 定义架构中间层接口（依赖倒置）。
//
// 本层只定义接口与错误哨兵，不包含任何实现，
// 保证内层领域逻辑与具体存储/虚拟化实现解耦。
package ports

import (
	"context"
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

// RoleRepository 角色（权限矩阵）数据仓储接口。
// 由 adapters 层实现，供权限管理与权限校验使用。
type RoleRepository interface {
	// Save 保存角色（存在则覆盖，upsert语义）
	Save(role *domain.RoleDefinition) error
	// FindByID 按ID查找角色，不存在返回 ErrNotFound
	FindByID(id string) (*domain.RoleDefinition, error)
	// FindAll 返回全部角色，按 ID 排序
	FindAll() ([]*domain.RoleDefinition, error)
	// Delete 按ID删除角色，不存在返回 ErrNotFound
	Delete(id string) error
}

// PermissionRepository 权限字典数据仓储接口。
// 由 adapters 层实现，供权限字典管理（角色编辑/插件声明校验）使用。
type PermissionRepository interface {
	// FindAll 返回全部权限字典条目，按 ID 排序
	FindAll() ([]*domain.PermissionInfo, error)
	// Exists 判断权限是否在字典中
	Exists(id string) (bool, error)
	// EnsureDefault 幂等写入默认权限字典（缺失才写入，已存在的不覆盖）
	EnsureDefault(perms []domain.PermissionInfo) error
}

// VMAccessRepository 虚拟机资源级授权仓储接口。
// 授权语义：记录 = 该用户可访问该 VM；管理员（vm:manage）不受此表限制。
type VMAccessRepository interface {
	// SetForUser 全量替换用户的可见 VM 集合（空列表 = 清空授权）
	SetForUser(userID string, vmIDs []string) error
	// VisibleVMIDs 返回用户全部可见 VM 的 ID 列表
	VisibleVMIDs(userID string) ([]string, error)
	// IsAuthorized 判断用户是否被授权访问指定 VM
	IsAuthorized(userID, vmID string) (bool, error)
	// DeleteForUser 删除用户的全部授权记录（删除用户时清理）
	DeleteForUser(userID string) error
}

// CaddyApplier 插件 Caddy 规则应用器（access 插件专属，由 pluginhost.CaddyManager 实现）。
// 提供落盘与 reload 两个步骤，便于调用方区分"落盘失败"与"reload 失败"。
type CaddyApplier interface {
	// Apply 写入/更新某插件的规则文件；rules 为空时等同 Remove。落盘失败返回错误。
	Apply(id string, rules string) error
	// Remove 删除某插件的规则文件（文件不存在时静默成功）。
	Remove(id string) error
	// Reload 触发 Caddy 热加载；命令未配置或不可用时静默成功（仅落盘）。
	Reload() error
	// HasRuleFile 判断某插件的规则文件当前是否已落盘（access 插件，供管理界面展示状态）。
	HasRuleFile(id string) bool
}

// NativeHost native 插件进程宿主接口（由 pluginhost.Manager 实现）。
// 管理 API 经此接口触发生命周期操作；状态查询供管理界面展示。
type NativeHost interface {
	// Enable 启用插件：更新 is_active=true 并 spawn 进程（native 专属）。
	Enable(ctx context.Context, id string) error
	// Disable 停用插件：更新 is_active=false 并停止进程（native 专属）。
	Disable(ctx context.Context, id string) error
	// Restart 重启插件：停止后重新 spawn（仅启用且已安装的插件生效）。
	Restart(ctx context.Context, id string) error
	// Status 返回插件运行态（running/stopped/error/missing）；未找到返回空串。
	Status(id string) string
	// HTTPAddress 返回插件 HTTP 数据面回环地址（"127.0.0.1:<port>"）；
	// 未运行返回空串。代理层据此反代（防 SSRF）。
	HTTPAddress(id string) string
}
