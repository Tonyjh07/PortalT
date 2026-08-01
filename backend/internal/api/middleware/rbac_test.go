package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"portalt/internal/domain"
)

func setupRBAC(user *domain.User) (*gin.Engine, *httptest.ResponseRecorder) {
	r, w := setupGin()
	tm := &stubTokenManager{user: user}
	r.GET("/admin", AuthRequired(tm),
		RequirePermission(domain.PERM_VM_MANAGE),
		func(c *gin.Context) { c.Status(http.StatusOK) })
	return r, w
}

func authReq(path string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer ok")
	return req
}

func TestRequirePermission_Granted(t *testing.T) {
	r, w := setupRBAC(&domain.User{ID: "u-1", Role: domain.RoleAdmin})
	r.ServeHTTP(w, authReq("/admin"))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequirePermission_Denied(t *testing.T) {
	r, w := setupRBAC(&domain.User{ID: "u-1", Role: domain.RoleViewer})
	r.ServeHTTP(w, authReq("/admin"))
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "权限不足")
}

func TestRequirePermission_NoUser(t *testing.T) {
	// 未经过认证（无用户上下文）时一律拒绝
	gin.SetMode(gin.TestMode)
	r, w := setupGin()
	r.GET("/admin", RequirePermission(domain.PERM_VM_MANAGE),
		func(c *gin.Context) { c.Status(http.StatusOK) })

	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/admin", nil))
	assert.Equal(t, http.StatusForbidden, w.Code)
}
