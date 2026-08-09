// Package v1 提供 /api/v1 接口处理器。
package v1

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"

	"portalt/internal/api/middleware"
	"portalt/internal/api/response"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

// 闸口读取的会话 cookie 名（与前端 frontend/utils/cookies.ts 保持一致）。
const (
	gateAccessCookie  = "access_token"
	gateRefreshCookie = "refresh_token"
)

// AuthHandler 认证相关接口处理器。
type AuthHandler struct {
	auth   ports.AuthenticationProvider
	tm     ports.TokenManager
	loader *middleware.RoleLoader
}

// NewAuthHandler 创建认证处理器。
// loader 为角色权限加载器（nil 时权限判定回退 domain.User.HasPermission 内置表，
// 用于无角色矩阵的测试与兜底路径）。
func NewAuthHandler(auth ports.AuthenticationProvider, tm ports.TokenManager, loader *middleware.RoleLoader) *AuthHandler {
	return &AuthHandler{auth: auth, tm: tm, loader: loader}
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

// Gate GET /api/v1/auth/gate?perm=<permission>
// Caddy forward_auth 回调闸口：校验访问令牌并检查指定权限，决定是否放行目标请求
// （如 ESXi 管理界面 /esxi/* 等路径，见 pluginhost.DefaultESXIAdminCaddyRules）。
// 令牌来源按序：Authorization: Bearer → ?token= → access_token cookie；
// access 失效/缺失时回退 refresh_token cookie（双令牌续期，配合前端插件页静默续期）。
// 权限判定优先使用角色矩阵（RoleLoader，运行时单一事实来源），与 RequirePermission 口径一致。
// 成功返回 200；未认证返回 401、无权限返回 403，均为浏览器可读的中文 HTML 提示页。
func (h *AuthHandler) Gate(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	user := h.gateUser(c)
	if user == nil {
		c.Data(http.StatusUnauthorized, "text/html; charset=utf-8",
			gateDeniedHTML("未登录或登录已过期", "请在门户登录后访问 ESXi 管理界面。"))
		return
	}
	if !h.gateHasPerm(user, c.Query("perm")) {
		c.Data(http.StatusForbidden, "text/html; charset=utf-8",
			gateDeniedHTML("无访问权限", "当前账号未授予 esxi-admin:use 权限，无法访问 ESXi 管理界面。"))
		return
	}
	c.Status(http.StatusOK)
}

// gateUser 按序解析闸口请求的登录用户；均失败返回 nil。
func (h *AuthHandler) gateUser(c *gin.Context) *domain.User {
	const prefix = "Bearer "
	if header := c.GetHeader("Authorization"); strings.HasPrefix(header, prefix) && len(header) > len(prefix) {
		if user, err := h.tm.ParseAccessToken(strings.TrimPrefix(header, prefix)); err == nil {
			return user
		}
	}
	if t := c.Query("token"); t != "" {
		// 与 AuthRequired 一致：guacamole/WS 拼接的 "?undefined" 等非 JWT 字符直接截断
		if i := strings.IndexAny(t, "?&"); i >= 0 {
			t = t[:i]
		}
		if user, err := h.tm.ParseAccessToken(t); err == nil {
			return user
		}
	}
	if t, err := c.Cookie(gateAccessCookie); err == nil && t != "" {
		if user, err := h.tm.ParseAccessToken(t); err == nil {
			return user
		}
	}
	// access 过期/缺失时回退刷新令牌续期。refresh token 本身就是有效会话凭据
	// （无状态、无法吊销，与 /auth/refresh 同等身份语义），此处额外做权限校验，比
	// refresh 端点更严格，故直接放行不构成提权。
	if t, err := c.Cookie(gateRefreshCookie); err == nil && t != "" {
		if user, err := h.tm.ParseRefreshToken(t); err == nil {
			return user
		}
	}
	return nil
}

// gateHasPerm 判定用户是否持有指定权限；perm 为空视为拒绝。
func (h *AuthHandler) gateHasPerm(user *domain.User, perm string) bool {
	if perm == "" {
		return false
	}
	if h.loader != nil {
		for _, p := range h.loader.PermissionsFor(user.Role) {
			if p == perm {
				return true
			}
		}
		return false
	}
	return user.HasPermission(perm)
}

// gateDeniedHTML 生成闸口拒绝页（未登录/无权限时在 iframe 内呈现，替代空白）。
func gateDeniedHTML(title, desc string) []byte {
	return []byte(fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh-CN">
<head><meta charset="utf-8"><title>%s</title></head>
<body style="font-family:system-ui,-apple-system,'Segoe UI',sans-serif;background:#0f1720;color:#e5e7eb;display:flex;align-items:center;justify-content:center;height:100vh;margin:0">
<div style="text-align:center">
<h2>%s</h2>
<p style="color:#9ca3af">%s</p>
<p><a href="/login" target="_top" style="color:#60a5fa">前往门户登录</a></p>
</div>
</body>
</html>`, title, title, desc))
}
