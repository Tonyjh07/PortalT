package middleware

import (
	"sync"

	"github.com/gin-gonic/gin"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

// permsContextKey 当前用户权限集合存入 gin.Context 的键。
const permsContextKey = "auth.perms"

// RoleLoader 角色权限加载器：从 RoleRepository 读取权限矩阵并缓存。
// 权限检查的运行时单一事实来源（角色表），角色变更后调用 Invalidate 失效缓存。
type RoleLoader struct {
	roles ports.RoleRepository

	mu    sync.RWMutex
	cache map[domain.Role][]string
}

// NewRoleLoader 创建加载器并预载全部角色（启动时调用）。
func NewRoleLoader(roles ports.RoleRepository) *RoleLoader {
	l := &RoleLoader{roles: roles, cache: make(map[domain.Role][]string)}
	for _, r := range domain.DefaultRoles() {
		l.cache[domain.Role(r.ID)] = r.Permissions
	}
	return l
}

// PermissionsFor 返回指定角色的权限集合；角色不存在时返回 nil。
func (l *RoleLoader) PermissionsFor(role domain.Role) []string {
	l.mu.RLock()
	if perms, ok := l.cache[role]; ok {
		l.mu.RUnlock()
		return perms
	}
	l.mu.RUnlock()

	l.mu.Lock()
	defer l.mu.Unlock()
	if perms, ok := l.cache[role]; ok {
		return perms
	}
	got, err := l.roles.FindByID(string(role))
	if err != nil {
		return nil
	}
	l.cache[role] = got.Permissions
	return got.Permissions
}

// Invalidate 失效指定角色的缓存（角色权限变更后调用）。
func (l *RoleLoader) Invalidate(role domain.Role) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.cache, role)
}

// AttachPermissions 把当前用户的权限集合装入上下文（需在 AuthRequired 之后）。
// 未加载到权限时回退由 RequirePermission 的内置表判断兜底。
func AttachPermissions(loader *RoleLoader) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := CurrentUser(c)
		if user != nil && loader != nil {
			perms := loader.PermissionsFor(user.Role)
			set := make(map[string]struct{}, len(perms))
			for _, p := range perms {
				set[p] = struct{}{}
			}
			c.Set(permsContextKey, set)
		}
		c.Next()
	}
}

// CurrentPerms 从上下文取出当前用户权限集合；未加载时返回 nil。
func CurrentPerms(c *gin.Context) map[string]struct{} {
	v, ok := c.Get(permsContextKey)
	if !ok {
		return nil
	}
	set, ok := v.(map[string]struct{})
	if !ok {
		return nil
	}
	return set
}
