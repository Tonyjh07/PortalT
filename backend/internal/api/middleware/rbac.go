package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"portalt/internal/api/response"
)

// RequirePermission 权限中间件：当前用户必须具备指定权限才可继续。
// 需在 AuthRequired 之后使用；权限常量见 internal/domain/permission.go。
func RequirePermission(perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := CurrentUser(c)
		if user == nil || !user.HasPermission(perm) {
			response.Error(c, http.StatusForbidden, response.CodeForbidden, "权限不足")
			return
		}
		c.Next()
	}
}
