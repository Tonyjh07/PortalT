// Package response 提供统一 HTTP 响应格式（独立叶子包，供 api 与中间件共用）。
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// 统一响应码（约定见 docs/interfaces.md）。
const (
	CodeSuccess = 200

	CodeInvalidCredentials = 4001 // 用户名或密码错误
	CodeInvalidToken       = 4002 // 令牌无效或已过期
	CodeMissingToken       = 4003 // 缺少访问令牌
	CodeBadRequest         = 4004 // 请求参数错误
	CodeForbidden          = 4005 // 权限不足
	CodeNotFound           = 4006 // 资源不存在
	CodeInvalidOperation   = 4007 // 操作在当前状态不允许
	CodeServerError        = 5000 // 服务器内部错误
)

// OK 成功响应：{code, message, data}
func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, gin.H{
		"code":    CodeSuccess,
		"message": "success",
		"data":    data,
	})
}

// Error 统一错误响应：{code, message}，可附 details。
func Error(c *gin.Context, httpStatus, code int, message string, details ...string) {
	body := gin.H{
		"code":    code,
		"message": message,
	}
	if len(details) > 0 && details[0] != "" {
		body["details"] = details[0]
	}
	c.AbortWithStatusJSON(httpStatus, body)
}
