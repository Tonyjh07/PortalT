package plugins

import (
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/domain"
)

var errNotFound = errors.New("not found")

type stubPlugin struct{ id string }

func (s stubPlugin) Info() domain.Plugin { return domain.Plugin{ID: s.id, Name: "stub-" + s.id} }
func (s stubPlugin) Mount(rt *gin.RouterGroup, deps Deps) {
	rt.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong-"+s.id) })
}
func (s stubPlugin) StaticFS() fs.FS { return nil }

type stubRepo struct {
	byID map[string]*domain.Plugin
	err  error
}

func (r *stubRepo) FindByID(id string) (*domain.Plugin, error) {
	if r.err != nil {
		return nil, r.err
	}
	if p, ok := r.byID[id]; ok {
		return p, nil
	}
	return nil, errNotFound
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, r.Register(stubPlugin{id: "a"}))
	require.NoError(t, r.Register(stubPlugin{id: "b"}))

	// ID 冲突拒绝
	err := r.Register(stubPlugin{id: "a"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "已注册")

	// nil 插件拒绝
	assert.Error(t, r.Register(nil))

	// 空 ID 拒绝
	assert.Error(t, r.Register(stubPlugin{id: ""}))

	_, ok := r.Get("a")
	assert.True(t, ok)
	_, ok = r.Get("nope")
	assert.False(t, ok)

	ids := make([]string, 0, 2)
	for _, p := range r.All() {
		ids = append(ids, p.Info().ID)
	}
	assert.Equal(t, []string{"a", "b"}, ids)
}

func TestRegistry_MountAPI_Gate(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.Register(stubPlugin{id: "demo"}))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	repo := &stubRepo{byID: map[string]*domain.Plugin{
		"demo": {ID: "demo", IsActive: true},
	}}
	reg.MountAPI(r.Group("/native"), Deps{}, repo)

	// 已启用 → 200
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/native/demo/ping", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "pong-demo", w.Body.String())

	// 停用 → 404
	repo.byID["demo"].IsActive = false
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/native/demo/ping", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "未启用")

	// 不存在 → 404
	repo.byID = map[string]*domain.Plugin{}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/native/demo/ping", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRegistry_MountStatic(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.Register(stubPlugin{id: "demo"}))
	// 无静态资源的插件不注册路由，访问应为 404
	gin.SetMode(gin.TestMode)
	r := gin.New()
	reg.MountStatic(r)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/native/demo/", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestNativeGate_NoRepo(t *testing.T) {
	// repo 返回错误 → 404 而非 500
	gin.SetMode(gin.TestMode)
	r := gin.New()
	reg := NewRegistry()
	require.NoError(t, reg.Register(stubPlugin{id: "demo"}))
	reg.MountAPI(r.Group("/native"), Deps{}, &stubRepo{err: errNotFound})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/native/demo/ping", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.True(t, strings.Contains(w.Body.String(), "插件不存在"))
}
