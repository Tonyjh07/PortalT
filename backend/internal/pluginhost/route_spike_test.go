package pluginhost

// 3.0 spike：验证 gin 动态路由两种候选方案的可行性，作为 Phase 3 路由设计依据。
//
// 结论记录（2026-08-08）：
//   - 方案 A（前缀占位路由 + manager 内部分发）可行：一条固定通配路由
//     /plugins/native/:pluginId/*path 即可承载任意插件，:pluginId 与 *path 可共存，
//     多插件不冲突，运行时无需重注册 gin 路由，天然适配热加载。
//   - 方案 B（启动前全量扫描重建路由）被否决：gin 对同前缀 handle 重复注册会 panic
//     （http: multiple registrations），动态增删路由需要重建 engine，复杂且易错。
//   - 结论：Phase 3 采用方案 A；本测试保留作为行为回归护栏。

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// TestGinRouteSpike_WildcardParamCoexists 验证 :pluginId 与 *path 通配参数在
// 一条路由上共存且都能取到值（方案 A 的核心可行性）。
func TestGinRouteSpike_WildcardParamCoexists(t *testing.T) {
	router := gin.New()
	var gotID, gotPath string
	router.Any("/api/v1/plugins/native/:pluginId/*path", func(c *gin.Context) {
		gotID = c.Param("pluginId")
		gotPath = c.Param("path")
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/plugins/native/hello/sub/x", nil)
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "hello", gotID)
	assert.Equal(t, "/sub/x", gotPath)
}

// TestGinRouteSpike_SamePrefixNoConflict 验证同一通配路由前缀下多个插件请求
// 互不冲突（只存在一条路由，无重复注册问题）。
func TestGinRouteSpike_SamePrefixNoConflict(t *testing.T) {
	router := gin.New()
	router.Any("/api/v1/plugins/native/:pluginId/*path", func(c *gin.Context) {
		c.String(http.StatusOK, "plugin="+c.Param("pluginId"))
	})

	for _, id := range []string{"alpha", "beta"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/plugins/native/"+id+"/do", nil)
		router.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "plugin="+id, w.Body.String())
	}
}

// TestGinRouteSpike_DuplicateRegistrationPanics 记录方案 B 被否决的根因：
// gin 对同前缀 handle 重复注册 panic（动态增删路由会触发）。
func TestGinRouteSpike_DuplicateRegistrationPanics(t *testing.T) {
	router := gin.New()
	group := router.Group("/plugins/native")

	register := func(id string) {
		g := group.Group("/" + id)
		g.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, id) })
	}

	register("alpha")
	require.NotPanics(t, func() {
		// 不同前缀：合法
		register("beta")
	})
	// 同前缀再次注册 → panic
	require.Panics(t, func() {
		register("alpha")
	})
}
