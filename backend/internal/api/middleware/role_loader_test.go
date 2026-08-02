package middleware

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/adapters/memory"
	"portalt/internal/domain"
	"portalt/internal/domain/services"
)

// TestRequirePermission_RoleMatrix 验证权限判定走角色矩阵（而非内置表）。
func TestRequirePermission_RoleMatrix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := memory.NewRoleRepository()
	require.NoError(t, services.EnsureDefaultRoles(t.Context(), repo))
	loader := NewRoleLoader(repo)

	// 把 viewer 的权限矩阵改为可启动虚拟机，验证矩阵优先于内置表
	viewer, err := repo.FindByID(string(domain.RoleViewer))
	require.NoError(t, err)
	viewer.Permissions = append(viewer.Permissions, domain.PERM_VM_START)
	require.NoError(t, repo.Save(viewer))
	loader.Invalidate(domain.RoleViewer)

	r, w := setupGin()
	tm := &stubTokenManager{user: &domain.User{ID: "u-1", Role: domain.RoleViewer}}
	r.GET("/start", AuthRequired(tm),
		AttachPermissions(loader),
		RequirePermission(domain.PERM_VM_START),
		func(c *gin.Context) { c.Status(http.StatusOK) })

	r.ServeHTTP(w, authReq("/start"))
	assert.Equal(t, http.StatusOK, w.Code)

	// 矩阵里没有的权限仍拒绝
	r2, w2 := setupGin()
	r2.GET("/manage", AuthRequired(tm),
		AttachPermissions(loader),
		RequirePermission(domain.PERM_USER_MANAGE),
		func(c *gin.Context) { c.Status(http.StatusOK) })
	r2.ServeHTTP(w2, authReq("/manage"))
	assert.Equal(t, http.StatusForbidden, w2.Code)
}

// TestRoleLoader_Invalidate 验证角色变更后缓存失效。
func TestRoleLoader_Invalidate(t *testing.T) {
	repo := memory.NewRoleRepository()
	require.NoError(t, services.EnsureDefaultRoles(t.Context(), repo))
	loader := NewRoleLoader(repo)

	assert.False(t, contains(loader.PermissionsFor(domain.RoleViewer), domain.PERM_VM_START))

	viewer, err := repo.FindByID(string(domain.RoleViewer))
	require.NoError(t, err)
	viewer.Permissions = []string{domain.PERM_VM_START}
	require.NoError(t, repo.Save(viewer))

	// 未失效前仍是旧值（缓存）
	assert.False(t, contains(loader.PermissionsFor(domain.RoleViewer), domain.PERM_VM_START))
	loader.Invalidate(domain.RoleViewer)
	assert.True(t, contains(loader.PermissionsFor(domain.RoleViewer), domain.PERM_VM_START))
}

// TestRoleLoader_UnknownRole 未知角色返回 nil 权限。
func TestRoleLoader_UnknownRole(t *testing.T) {
	repo := memory.NewRoleRepository()
	loader := NewRoleLoader(repo)
	assert.Nil(t, loader.PermissionsFor(domain.Role("ghost")))
}

func contains(perms []string, perm string) bool {
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}
