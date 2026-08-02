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

// AuthRequired 解析访问令牌：
//  1. Authorization: Bearer <token>（常规 API 请求）
//  2. ?token=<token>（WebSocket 升级请求无法携带自定义请求头）
//
// 成功后把 *domain.User 存入上下文，供后续处理器使用。
func AuthRequired(tm ports.TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		const prefix = "Bearer "
		token := ""
		if strings.HasPrefix(header, prefix) && len(header) > len(prefix) {
			token = strings.TrimPrefix(header, prefix)
		}
		if token == "" {
			token = c.Query("token")
			// guacamole-common-js 的 WebSocketTunnel 会在 URL 后追加 "?" + connect 数据
			// （无参时形如 "?undefined"），这些字符不属 JWT 字符集，直接截断。
			if i := strings.IndexAny(token, "?&"); i >= 0 {
				token = token[:i]
			}
		}
		if token == "" {
			response.Error(c, http.StatusUnauthorized, response.CodeMissingToken, "缺少访问令牌")
			return
		}

		user, err := tm.ParseAccessToken(token)
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
