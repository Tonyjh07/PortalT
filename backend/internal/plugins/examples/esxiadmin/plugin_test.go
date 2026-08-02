package esxiadmin

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

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
