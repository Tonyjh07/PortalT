package esxiadmin

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/domain"
	"portalt/internal/plugins"
)

func setupRouter(provider, webURL string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("auth.user", &domain.User{ID: "u1", Username: "admin", Role: domain.RoleAdmin})
		c.Next()
	})
	p := New()
	p.Mount(r.Group("/plugins/native/esxi-admin"), plugins.Deps{Provider: provider, WebURL: webURL})
	return r
}

func TestEsxiAdmin_Config(t *testing.T) {
	r := setupRouter("mock", "")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/plugins/native/esxi-admin/config", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"provider":"mock"`)
	assert.Contains(t, w.Body.String(), `"connected":false`)
	assert.Contains(t, w.Body.String(), `"web_url":""`)

	r2 := setupRouter("esxi", "https://esxi.lan/ui/")
	w = httptest.NewRecorder()
	r2.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/plugins/native/esxi-admin/config", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"provider":"esxi"`)
	assert.Contains(t, w.Body.String(), `"connected":true`)
	assert.Contains(t, w.Body.String(), `"web_url":"https://esxi.lan/ui/"`)
}

func TestEsxiAdmin_StaticFS(t *testing.T) {
	p := New()
	fsys := p.StaticFS()
	assert.NotNil(t, fsys, "插件应提供内嵌静态前端")
	b, err := fs.ReadFile(fsys, "index.html")
	assert.NoError(t, err)
	assert.Contains(t, string(b), "esxiFrame")
	assert.Contains(t, string(b), "portalt-theme")
}

// TestInfo_DeclaresDedicatedPermission 确保插件声明专属权限：
// 值遵循「插件命名空间:操作」约定，且存在于权限字典、默认授予 admin 角色。
// 回归保护：若改回通用 plugin:view 或命名漂移，此处立即失败。
func TestInfo_DeclaresDedicatedPermission(t *testing.T) {
	info := New().Info()
	require.Equal(t, domain.PERM_ESXI_ADMIN_USE, info.Permission)
	assert.NotEqual(t, domain.PERM_PLUGIN_VIEW, info.Permission)

	// 声明值必须在权限字典内（否则角色管理 API 与 nativeGate 校验无法通过）
	dict := domain.AllPermissions()
	ids := make([]string, 0, len(dict))
	for _, p := range dict {
		ids = append(ids, p.ID)
	}
	assert.Contains(t, ids, info.Permission)

	// 默认授予 admin 角色，普通用户/访客默认不持有
	for _, r := range domain.DefaultRoles() {
		switch r.ID {
		case string(domain.RoleAdmin):
			assert.Contains(t, r.Permissions, info.Permission)
		default:
			assert.NotContains(t, r.Permissions, info.Permission)
		}
	}
}
