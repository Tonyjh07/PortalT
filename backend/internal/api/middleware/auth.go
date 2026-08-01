// Package middleware 提供 HTTP 中间件：认证、权限等。
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"portalt/internal/api/response"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

// userContextKey 当前用户存入 gin.Context 的键。
const userContextKey = "auth.user"

// AuthRequired 从 Authorization: Bearer <token> 解析访问令牌，
// 成功后把 *domain.User 存入上下文，供后续处理器使用。
func AuthRequired(tm ports.TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(header, prefix) || len(header) <= len(prefix) {
			response.Error(c, http.StatusUnauthorized, response.CodeMissingToken, "缺少访问令牌")
			return
		}

		user, err := tm.ParseAccessToken(strings.TrimPrefix(header, prefix))
		if err != nil {
			response.Error(c, http.StatusUnauthorized, response.CodeInvalidToken, "令牌无效或已过期")
			return
		}
		c.Set(userContextKey, user)
		c.Next()
	}
}

// CurrentUser 从上下文取出当前用户；未经认证时返回 nil。
func CurrentUser(c *gin.Context) *domain.User {
	v, ok := c.Get(userContextKey)
	if !ok {
		return nil
	}
	user, ok := v.(*domain.User)
	if !ok {
		return nil
	}
	return user
}
