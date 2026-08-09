package v1

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authadapter "portalt/internal/adapters/auth"
	"portalt/internal/adapters/memory"
	"portalt/internal/api/middleware"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

const gateTestSecret = "gate-test-secret"

// newGateEnv 组装闸口测试环境（JWT 管理器 + 角色矩阵加载器）。
// loader 为 nil 时走 domain.User.HasPermission 内置表回退路径。
func newGateEnv(t *testing.T, loader *middleware.RoleLoader) (ports.TokenManager, *AuthHandler) {
	t.Helper()
	tm := authadapter.NewJWTManager(gateTestSecret, 15*time.Minute, 7*24*time.Hour)
	return tm, NewAuthHandler(nil, tm, loader)
}

// gateReq 构造闸口请求并执行，返回响应记录器。
func gateReq(t *testing.T, h *AuthHandler, perm string, reqFn func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/gate?perm="+perm, nil)
	if reqFn != nil {
		reqFn(req)
	}
	c.Request = req
	h.Gate(c)
	return w
}

func mustAccessToken(t *testing.T, tm ports.TokenManager, user *domain.User) string {
	t.Helper()
	s, err := tm.GenerateAccessToken(user)
	require.NoError(t, err)
	return s
}

func mustRefreshToken(t *testing.T, tm ports.TokenManager, user *domain.User) string {
	t.Helper()
	s, err := tm.GenerateRefreshToken(user)
	require.NoError(t, err)
	return s
}

// expiredAccessToken 用同一密钥签发一份已过期的访问令牌（与 JWTManager 同结构）。
func expiredAccessToken(t *testing.T, user *domain.User) string {
	t.Helper()
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"username": user.Username,
		"role":     string(user.Role),
		"type":     authadapter.TokenTypeAccess,
		"sub":      user.ID,
		"iss":      "portalt",
		"iat":      now.Add(-2 * time.Hour).Unix(),
		"nbf":      now.Add(-2 * time.Hour).Unix(),
		"exp":      now.Add(-1 * time.Hour).Unix(),
	})
	s, err := token.SignedString([]byte(gateTestSecret))
	require.NoError(t, err)
	return s
}

func adminUser() *domain.User {
	return &domain.User{ID: "u-admin", Username: "admin", Role: domain.RoleAdmin}
}

func viewerUser() *domain.User {
	return &domain.User{ID: "u-viewer", Username: "viewer", Role: domain.RoleViewer}
}

func TestGate_NoToken(t *testing.T) {
	tm, h := newGateEnv(t, middleware.NewRoleLoader(memory.NewRoleRepository()))
	_ = tm
	w := gateReq(t, h, domain.PERM_ESXI_ADMIN_USE, nil)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
}

func TestGate_ValidAccess_AdminAllowed(t *testing.T) {
	tm, h := newGateEnv(t, middleware.NewRoleLoader(memory.NewRoleRepository()))
	w := gateReq(t, h, domain.PERM_ESXI_ADMIN_USE, func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+mustAccessToken(t, tm, adminUser()))
	})
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGate_ValidAccess_ViewerDenied(t *testing.T) {
	tm, h := newGateEnv(t, middleware.NewRoleLoader(memory.NewRoleRepository()))
	w := gateReq(t, h, domain.PERM_ESXI_ADMIN_USE, func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+mustAccessToken(t, tm, viewerUser()))
	})
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "无访问权限")
}

func TestGate_ViewerOwnsOtherPerm(t *testing.T) {
	tm, h := newGateEnv(t, middleware.NewRoleLoader(memory.NewRoleRepository()))
	w := gateReq(t, h, domain.PERM_VM_VIEW, func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+mustAccessToken(t, tm, viewerUser()))
	})
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGate_AccessFromCookie(t *testing.T) {
	tm, h := newGateEnv(t, middleware.NewRoleLoader(memory.NewRoleRepository()))
	w := gateReq(t, h, domain.PERM_ESXI_ADMIN_USE, func(req *http.Request) {
		req.AddCookie(&http.Cookie{Name: "access_token", Value: mustAccessToken(t, tm, adminUser())})
	})
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGate_HeaderPreferredOverCookie(t *testing.T) {
	tm, h := newGateEnv(t, middleware.NewRoleLoader(memory.NewRoleRepository()))
	// cookie 为无权限的 viewer，header 为有权限的 admin：应以 header 为准
	w := gateReq(t, h, domain.PERM_ESXI_ADMIN_USE, func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+mustAccessToken(t, tm, adminUser()))
		req.AddCookie(&http.Cookie{Name: "access_token", Value: mustAccessToken(t, tm, viewerUser())})
	})
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGate_TokenFromQuery(t *testing.T) {
	tm, h := newGateEnv(t, middleware.NewRoleLoader(memory.NewRoleRepository()))
	token := mustAccessToken(t, tm, adminUser())
	w := gateReq(t, h, domain.PERM_ESXI_ADMIN_USE, func(req *http.Request) {
		req.URL.RawQuery = "perm=" + domain.PERM_ESXI_ADMIN_USE + "&token=" + token
	})
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGate_TokenQuery_Truncated(t *testing.T) {
	tm, h := newGateEnv(t, middleware.NewRoleLoader(memory.NewRoleRepository()))
	token := mustAccessToken(t, tm, adminUser())
	// 模拟 WS 拼接的 "?undefined"：token 应按 ?/& 截断后仍可解析
	w := gateReq(t, h, domain.PERM_ESXI_ADMIN_USE, func(req *http.Request) {
		req.URL.RawQuery = "perm=" + domain.PERM_ESXI_ADMIN_USE + "&token=" + token + "?undefined"
	})
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGate_ExpiredAccess_RefreshFallback(t *testing.T) {
	tm, h := newGateEnv(t, middleware.NewRoleLoader(memory.NewRoleRepository()))
	w := gateReq(t, h, domain.PERM_ESXI_ADMIN_USE, func(req *http.Request) {
		req.AddCookie(&http.Cookie{Name: "access_token", Value: expiredAccessToken(t, adminUser())})
		req.AddCookie(&http.Cookie{Name: "refresh_token", Value: mustRefreshToken(t, tm, adminUser())})
	})
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGate_RefreshFallback_ViewerDenied(t *testing.T) {
	tm, h := newGateEnv(t, middleware.NewRoleLoader(memory.NewRoleRepository()))
	w := gateReq(t, h, domain.PERM_ESXI_ADMIN_USE, func(req *http.Request) {
		req.AddCookie(&http.Cookie{Name: "access_token", Value: expiredAccessToken(t, viewerUser())})
		req.AddCookie(&http.Cookie{Name: "refresh_token", Value: mustRefreshToken(t, tm, viewerUser())})
	})
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGate_InvalidToken(t *testing.T) {
	_, h := newGateEnv(t, middleware.NewRoleLoader(memory.NewRoleRepository()))
	w := gateReq(t, h, domain.PERM_ESXI_ADMIN_USE, func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer garbage-token")
	})
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestGate_MissingPermParam(t *testing.T) {
	tm, h := newGateEnv(t, middleware.NewRoleLoader(memory.NewRoleRepository()))
	w := gateReq(t, h, "", func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+mustAccessToken(t, tm, adminUser()))
	})
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestGate_FallbackHasPermission_NoLoader(t *testing.T) {
	tm, h := newGateEnv(t, nil) // loader=nil → 回退内置角色表
	w := gateReq(t, h, domain.PERM_ESXI_ADMIN_USE, func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+mustAccessToken(t, tm, adminUser()))
	})
	assert.Equal(t, http.StatusOK, w.Code)

	w2 := gateReq(t, h, domain.PERM_ESXI_ADMIN_USE, func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+mustAccessToken(t, tm, viewerUser()))
	})
	assert.Equal(t, http.StatusForbidden, w2.Code)
}

func TestGate_MatrixDrivenByRoleLoader(t *testing.T) {
	// 自定义角色矩阵：viewer 被授予 esxi-admin:use 后即可放行
	repo := memory.NewRoleRepository()
	require.NoError(t, repo.Save(&domain.RoleDefinition{
		ID:          string(domain.RoleViewer),
		Name:        "访客",
		Permissions: []string{domain.PERM_ESXI_ADMIN_USE},
	}))
	loader := middleware.NewRoleLoader(repo)
	loader.Invalidate(domain.RoleViewer) // 清掉默认缓存，读取自定义矩阵
	tm, h := newGateEnv(t, loader)

	w := gateReq(t, h, domain.PERM_ESXI_ADMIN_USE, func(req *http.Request) {
		req.Header.Set("Authorization", "Bearer "+mustAccessToken(t, tm, viewerUser()))
	})
	assert.Equal(t, http.StatusOK, w.Code)
	assert.False(t, strings.Contains(w.Body.String(), "无访问权限"))
}
