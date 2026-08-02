package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/adapters/memory"
	"portalt/internal/domain"
)

func TestEnsureDefaultRoles_SeedsAndIdempotent(t *testing.T) {
	repo := memory.NewRoleRepository()
	ctx := t.Context()

	require.NoError(t, EnsureDefaultRoles(ctx, repo))
	all, err := repo.FindAll()
	require.NoError(t, err)
	require.Len(t, all, 3)

	admin, err := repo.FindByID(string(domain.RoleAdmin))
	require.NoError(t, err)
	assert.Equal(t, "管理员", admin.Name)
	assert.Contains(t, admin.Permissions, domain.PERM_USER_MANAGE)

	// 幂等：再次执行不重复、不覆盖
	admin.Permissions = []string{domain.PERM_VM_VIEW}
	require.NoError(t, repo.Save(admin))
	require.NoError(t, EnsureDefaultRoles(ctx, repo))
	again, err := repo.FindByID(string(domain.RoleAdmin))
	require.NoError(t, err)
	assert.Equal(t, []string{domain.PERM_VM_VIEW}, again.Permissions)
}
