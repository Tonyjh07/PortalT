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
	// PERM_VM_CONSOLE 访问虚拟机远程桌面与控制台（与 vm:view 分离，可单独授予）
	PERM_VM_CONSOLE = "vm:console"
	// PERM_PLUGIN_VIEW 查看插件菜单
	PERM_PLUGIN_VIEW = "plugin:view"
	// PERM_PLUGIN_MANAGE 管理插件（增删改）
	PERM_PLUGIN_MANAGE = "plugin:manage"
	// PERM_USER_MANAGE 管理用户
	PERM_USER_MANAGE = "user:manage"
	// PERM_ESXI_ADMIN_USE 访问 ESXi 管理界面（esxi-admin 插件）。
	// 控制插件访问（access 代理/页面经 CanAccess 校验）；菜单入口组级仍要求通用
	// plugin:view（/menu），默认 admin 双持有，自定义角色
	// 最小授权时建议同时授予 plugin:view。
	PERM_ESXI_ADMIN_USE = "esxi-admin:use"
	// PERM_FRPC_ADMIN_MANAGE 管理 frpc 配置（frpc-admin 插件：配置 SSH 连接、
	// 读写 frpc 配置、保存并重启）。默认仅 admin 持有；自定义角色如需使用
	// 该插件须同时授予 plugin:view。
	PERM_FRPC_ADMIN_MANAGE = "frpc-admin:manage"
)
