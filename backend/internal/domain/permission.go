package domain

// 权限常量定义。
// 命名规范：使用 "资源:动作" 形式，与插件表的 permission 字段及
// RBAC 中间件的权限检查保持一致的字符串约定。
const (
	// PERM_VIEW_ALL 查看门户内所有资源
	PERM_VIEW_ALL = "view_all"
	// PERM_VM_VIEW 查看虚拟机列表与详情
	PERM_VM_VIEW = "vm:view"
	// PERM_VM_START 启动虚拟机
	PERM_VM_START = "vm:start"
	// PERM_VM_STOP 停止虚拟机
	PERM_VM_STOP = "vm:stop"
	// PERM_VM_RESTART 重启虚拟机
	PERM_VM_RESTART = "vm:restart"
	// PERM_VM_MANAGE 管理虚拟机（增删改）
	PERM_VM_MANAGE = "vm:manage"
	// PERM_PLUGIN_VIEW 查看插件菜单
	PERM_PLUGIN_VIEW = "plugin:view"
	// PERM_PLUGIN_MANAGE 管理插件（增删改）
	PERM_PLUGIN_MANAGE = "plugin:manage"
	// PERM_USER_MANAGE 管理用户
	PERM_USER_MANAGE = "user:manage"
)
