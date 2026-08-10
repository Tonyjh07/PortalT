package domain

// Role 用户角色类型
type Role string

const (
	// RoleAdmin 管理员：拥有全部权限，可管理用户与插件
	RoleAdmin Role = "admin"
	// RoleUser 普通用户：可查看并操作虚拟机
	RoleUser Role = "user"
	// RoleViewer 访客：仅可查看，无操作权限
	RoleViewer Role = "viewer"
)

// rolePermissions 角色到权限集合的映射表。
// 使用 map 结构以便 O(1) 查询，权限检查依赖此表。
var rolePermissions = map[Role]map[string]struct{}{
	RoleAdmin: {
		PERM_VIEW_ALL:       {},
		PERM_VM_VIEW:        {},
		PERM_VM_START:       {},
		PERM_VM_STOP:        {},
		PERM_VM_RESTART:     {},
		PERM_VM_MANAGE:      {},
		PERM_VM_CONSOLE:     {},
		PERM_PLUGIN_VIEW:    {},
		PERM_PLUGIN_MANAGE:  {},
		PERM_USER_MANAGE:    {},
		PERM_ESXI_ADMIN_USE: {},
		PERM_FRPC_ADMIN_MANAGE: {},
	},
	RoleUser: {
		PERM_VIEW_ALL:    {},
		PERM_VM_VIEW:     {},
		PERM_VM_START:    {},
		PERM_VM_STOP:     {},
		PERM_VM_RESTART:  {},
		PERM_VM_CONSOLE:  {},
		PERM_PLUGIN_VIEW: {},
	},
	RoleViewer: {
		PERM_VIEW_ALL: {},
		PERM_VM_VIEW:  {},
	},
}

// User 用户领域实体，描述门户的一个用户账号。
type User struct {
	// ID 唯一标识
	ID string `json:"id"`
	// Username 用户名（登录凭证）
	Username string `json:"username"`
	// Password 密码哈希，JSON序列化时隐藏
	Password string `json:"-"`
	// Email 邮箱
	Email string `json:"email"`
	// Role 用户角色
	Role Role `json:"role"`
}

// HasPermission 判断用户是否拥有指定权限。
// 未注册的角色（空或其他值）一律视为无任何权限。
func (u *User) HasPermission(perm string) bool {
	if u == nil {
		return false
	}
	perms, ok := rolePermissions[u.Role]
	if !ok {
		return false
	}
	_, ok = perms[perm]
	return ok
}

// IsAdmin 判断用户是否为管理员。
func (u *User) IsAdmin() bool {
	return u != nil && u.Role == RoleAdmin
}
