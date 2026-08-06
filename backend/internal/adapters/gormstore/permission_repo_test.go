package gormstore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/domain"
)

func TestPermissionRepository_EnsureDefault_Idempotent(t *testing.T) {
	db := newTestDB(t)
	repo := NewPermissionRepository(db)

	require.NoError(t, repo.EnsureDefault(domain.AllPermissions()))

	all, err := repo.FindAll()
	require.NoError(t, err)
	assert.Len(t, all, len(domain.AllPermissions()))

	// 二次执行不重复插入
	require.NoError(t, repo.EnsureDefault(domain.AllPermissions()))
	all, err = repo.FindAll()
	require.NoError(t, err)
	assert.Len(t, all, len(domain.AllPermissions()))

	ok, err := repo.Exists(domain.PERM_VM_CONSOLE)
	require.NoError(t, err)
	assert.True(t, ok)

	ok, err = repo.Exists("nope:perm")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestVMAccessRepository_CRUD(t *testing.T) {
	db := newTestDB(t)
	repo := NewVMAccessRepository(db)

	require.NoError(t, repo.SetForUser("u1", []string{"vm-a", "vm-b"}))

	ids, err := repo.VisibleVMIDs("u1")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"vm-a", "vm-b"}, ids)

	// 覆盖写入（全量替换）
	require.NoError(t, repo.SetForUser("u1", []string{"vm-b", "vm-c"}))
	ids, err = repo.VisibleVMIDs("u1")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"vm-b", "vm-c"}, ids)

	// 授权/未授权判定
	ok, err := repo.IsAuthorized("u1", "vm-b")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = repo.IsAuthorized("u1", "vm-a")
	require.NoError(t, err)
	assert.False(t, ok)

	// 其他用户互不影响
	require.NoError(t, repo.SetForUser("u2", []string{"vm-a"}))
	ok, err = repo.IsAuthorized("u2", "vm-a")
	require.NoError(t, err)
	assert.True(t, ok)

	// 删除用户全部授权
	require.NoError(t, repo.DeleteForUser("u1"))
	ok, err = repo.IsAuthorized("u1", "vm-b")
	require.NoError(t, err)
	assert.False(t, ok)
}
