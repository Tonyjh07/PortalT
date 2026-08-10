package domain

import "sort"

// Role 角色领域实体：一组权限的集合。
// 角色与权限矩阵入库（roles 表），内置三角色作为种子数据，管理员可调整。
type RoleDefinition struct {
	// ID 角色标识（内置角色与 User.Role 值一致：admin/user/viewer）
	ID string `json:"id"`
	// Name 显示名称
	Name string `json:"name"`
	// Description 描述
	Description string `json:"description"`
	// Permissions 角色拥有的权限列表（去重）
	Permissions []string `json:"permissions"`
}

// roleNames 内置角色显示名。
var roleNames = map[Role]string{
	RoleAdmin:  "管理员",
	RoleUser:   "普通用户",
	RoleViewer: "访客",
}

// roleDescriptions 内置角色描述。
var roleDescriptions = map[Role]string{
	RoleAdmin:  "拥有全部权限，可管理用户、角色与插件",
	RoleUser:   "可查看并操作虚拟机，查看插件",
	RoleViewer: "仅可查看虚拟机，无操作权限",
}

// DefaultRoles 返回内置默认角色定义。
// 以 rolePermissions 静态表为单一事实来源生成，保证种子数据与代码基线一致。
func DefaultRoles() []*RoleDefinition {
	order := []Role{RoleAdmin, RoleUser, RoleViewer}
	roles := make([]*RoleDefinition, 0, len(order))
	for _, r := range order {
		perms := make([]string, 0, len(rolePermissions[r]))
		for p := range rolePermissions[r] {
			perms = append(perms, p)
		}
		sort.Strings(perms)
		roles = append(roles, &RoleDefinition{
			ID:          string(r),
			Name:        roleNames[r],
			Description: roleDescriptions[r],
			Permissions: perms,
		})
	}
	return roles
}

// PermissionInfo 权限字典条目（供管理界面渲染）。
type PermissionInfo struct {
	// ID 权限标识（如 "vm:start"）
	ID string `json:"id"`
	// Description 中文描述
	Description string `json:"description"`
}

// permissionDescriptions 全部权限的中文描述。
var permissionDescriptions = map[string]string{
	PERM_VIEW_ALL:       "查看门户内所有资源",
	PERM_VM_VIEW:        "查看虚拟机列表与详情",
	PERM_VM_START:       "启动虚拟机",
	PERM_VM_STOP:        "停止虚拟机",
	PERM_VM_RESTART:     "重启虚拟机",
	PERM_VM_MANAGE:      "管理虚拟机（增删改）",
	PERM_VM_CONSOLE:     "访问虚拟机远程桌面与控制台",
	PERM_PLUGIN_VIEW:    "查看插件菜单并调用插件接口",
	PERM_PLUGIN_MANAGE:  "管理插件（增删改与启用停用）",
	PERM_USER_MANAGE:    "管理用户与角色权限",
	PERM_ESXI_ADMIN_USE: "访问 ESXi 管理界面（esxi-admin 插件）",
	PERM_FRPC_ADMIN_MANAGE: "管理 frpc 配置（frpc-admin 插件）",
}

// AllPermissions 返回系统全部可用权限字典，按 ID 排序（确定性输出）。
func AllPermissions() []PermissionInfo {
	ids := make([]string, 0, len(permissionDescriptions))
	for id := range permissionDescriptions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]PermissionInfo, 0, len(ids))
	for _, id := range ids {
		out = append(out, PermissionInfo{ID: id, Description: permissionDescriptions[id]})
	}
	return out
}

// IsBuiltinRole 判断角色是否为内置角色（admin/user/viewer）。
// 内置角色不允许删除，只允许调整权限矩阵。
func IsBuiltinRole(id string) bool {
	switch Role(id) {
	case RoleAdmin, RoleUser, RoleViewer:
		return true
	default:
		return false
	}
}

// HasPermissionWith 判断权限集合是否包含指定权限（顺序无关）。
func HasPermissionWith(perms []string, perm string) bool {
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}
