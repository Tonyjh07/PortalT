// Package v1 提供 /api/v1 接口处理器。
package v1

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"portalt/internal/api/middleware"
	"portalt/internal/api/response"
	"portalt/internal/ports"
)

// AuthHandler 认证相关接口处理器。
type AuthHandler struct {
	auth ports.AuthenticationProvider
	tm   ports.TokenManager
}

// NewAuthHandler 创建认证处理器。
func NewAuthHandler(auth ports.AuthenticationProvider, tm ports.TokenManager) *AuthHandler {
	return &AuthHandler{auth: auth, tm: tm}
}

// loginRequest 登录请求体。
type loginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// Login POST /api/v1/auth/login
// 校验凭据并签发访问令牌 + 刷新令牌。
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, "请求参数错误", err.Error())
		return
	}

	user, err := h.auth.Authenticate(req.Username, req.Password)
	if errors.Is(err, ports.ErrInvalidCredentials) {
		response.Error(c, http.StatusUnauthorized, response.CodeInvalidCredentials, "用户名或密码错误")
		return
	}
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "认证服务异常")
		return
	}

	access, err := h.tm.GenerateAccessToken(user)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "令牌签发失败")
		return
	}
	refresh, err := h.tm.GenerateRefreshToken(user)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "令牌签发失败")
		return
	}

	response.OK(c, gin.H{
		"access_token":  access,
		"refresh_token": refresh,
		"expires_in":    int(h.tm.AccessTTL().Seconds()),
		"user":          user,
	})
}

// refreshRequest 刷新令牌请求体。
type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// Refresh POST /api/v1/auth/refresh
// 用刷新令牌换取新的访问令牌（不轮换刷新令牌本身，简单模型）。
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, http.StatusBadRequest, response.CodeBadRequest, "请求参数错误", err.Error())
		return
	}

	user, err := h.tm.ParseRefreshToken(req.RefreshToken)
	if err != nil {
		response.Error(c, http.StatusUnauthorized, response.CodeInvalidToken, "刷新令牌无效或已过期")
		return
	}

	access, err := h.tm.GenerateAccessToken(user)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, response.CodeServerError, "令牌签发失败")
		return
	}

	response.OK(c, gin.H{
		"access_token": access,
		"expires_in":   int(h.tm.AccessTTL().Seconds()),
	})
}

// Me GET /api/v1/auth/me
// 返回当前登录用户信息（需认证，验证中间件链路）。
func (h *AuthHandler) Me(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		response.Error(c, http.StatusUnauthorized, response.CodeInvalidToken, "令牌无效或已过期")
		return
	}
	response.OK(c, user)
}
