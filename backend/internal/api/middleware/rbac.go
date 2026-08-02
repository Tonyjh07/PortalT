package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"portalt/internal/api/response"
)

// RequirePermission 权限中间件：当前用户必须具备指定权限才可继续。
// 需在 AuthRequired（及可选 AttachPermissions）之后使用；权限常量见 internal/domain/permission.go。
// 权限判定优先使用 AttachPermissions 写入上下文的权限集合（角色矩阵，运行时单一事实来源）；
// 未加载权限上下文时回退 domain.User.HasPermission 内置表（旧测试与兜底路径）。
func RequirePermission(perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := CurrentUser(c)
		if user == nil {
			response.Error(c, http.StatusForbidden, response.CodeForbidden, "权限不足")
			return
		}
		if perms := CurrentPerms(c); perms != nil {
			if _, ok := perms[perm]; !ok {
				response.Error(c, http.StatusForbidden, response.CodeForbidden, "权限不足")
				return
			}
			c.Next()
			return
		}
		if !user.HasPermission(perm) {
			response.Error(c, http.StatusForbidden, response.CodeForbidden, "权限不足")
			return
		}
		c.Next()
	}
}

// RequireAnyPermission 权限中间件：具备列表中任一权限即可继续（如用户管理或插件管理均可）。
func RequireAnyPermission(perms ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := CurrentUser(c)
		if user == nil {
			response.Error(c, http.StatusForbidden, response.CodeForbidden, "权限不足")
			return
		}
		loaded := CurrentPerms(c)
		for _, perm := range perms {
			if loaded != nil {
				if _, ok := loaded[perm]; ok {
					c.Next()
					return
				}
			} else if user.HasPermission(perm) {
				c.Next()
				return
			}
		}
		response.Error(c, http.StatusForbidden, response.CodeForbidden, "权限不足")
	}
}
