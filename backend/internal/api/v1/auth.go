// Package v1 提供 /api/v1 接口处理器。
package v1

import (
	"errors"
	"net/http"
	"slices"

	"github.com/gin-gonic/gin"

	"portalt/internal/api/middleware"
	"portalt/internal/api/response"
	"portalt/internal/domain"
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

// currentPermsList 返回当前用户权限列表（排序去重，确定性输出）。
// 优先使用 AttachPermissions 写入上下文的权限集合（角色矩阵，运行时单一事实来源）；
// 未加载权限上下文（如单元测试直连处理器）时回退用户内置角色表。
func currentPermsList(c *gin.Context) []string {
	perms := make([]string, 0, 8)
	if set := middleware.CurrentPerms(c); set != nil {
		for p := range set {
			perms = append(perms, p)
		}
	} else if user := middleware.CurrentUser(c); user != nil {
		for _, p := range domain.AllPermissions() {
			if user.HasPermission(p.ID) {
				perms = append(perms, p.ID)
			}
		}
	}
	slices.Sort(perms)
	return perms
}

// Me GET /api/v1/auth/me
// 返回当前登录用户信息与权限集合（需认证，验证中间件链路）。
func (h *AuthHandler) Me(c *gin.Context) {
	user := middleware.CurrentUser(c)
	if user == nil {
		response.Error(c, http.StatusUnauthorized, response.CodeInvalidToken, "令牌无效或已过期")
		return
	}
	response.OK(c, gin.H{
		"id":          user.ID,
		"username":    user.Username,
		"email":       user.Email,
		"role":        user.Role,
		"permissions": currentPermsList(c),
	})
}
