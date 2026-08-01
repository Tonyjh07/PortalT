//go:build integration

package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

func TestUserRepository_Crud(t *testing.T) {
	db := setupTestDB(t)
	truncateTables(t, db)
	repo := NewUserRepository(db)

	user := &domain.User{
		ID:       "u-1",
		Username: "alice",
		Password: "hash",
		Email:    "alice@lab.local",
		Role:     domain.RoleAdmin,
	}

	require.NoError(t, repo.Save(user))

	got, err := repo.FindByID("u-1")
	require.NoError(t, err)
	assert.Equal(t, user, got)

	// 密码更新
	user.Password = "new-hash"
	user.Role = domain.RoleUser
	require.NoError(t, repo.Save(user))
	got, err = repo.FindByID("u-1")
	require.NoError(t, err)
	assert.Equal(t, "new-hash", got.Password)
	assert.Equal(t, domain.RoleUser, got.Role)

	// 删除
	require.NoError(t, repo.Delete("u-1"))
	_, err = repo.FindByID("u-1")
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

func TestUserRepository_FindByUsername(t *testing.T) {
	db := setupTestDB(t)
	truncateTables(t, db)
	repo := NewUserRepository(db)

	require.NoError(t, repo.Save(&domain.User{ID: "u-1", Username: "bob", Password: "h", Role: domain.RoleViewer}))

	got, err := repo.FindByUsername("bob")
	require.NoError(t, err)
	assert.Equal(t, "u-1", got.ID)

	_, err = repo.FindByUsername("nobody")
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

func TestUserRepository_UsernameUnique(t *testing.T) {
	db := setupTestDB(t)
	truncateTables(t, db)
	repo := NewUserRepository(db)

	require.NoError(t, repo.Save(&domain.User{ID: "u-1", Username: "dup", Password: "h", Role: domain.RoleUser}))
	// 不同ID相同用户名应违反唯一约束
	err := repo.Save(&domain.User{ID: "u-2", Username: "dup", Password: "h", Role: domain.RoleUser})
	assert.Error(t, err)
}

func TestUserRepository_FindAll(t *testing.T) {
	db := setupTestDB(t)
	truncateTables(t, db)
	repo := NewUserRepository(db)

	require.NoError(t, repo.Save(&domain.User{ID: "u-2", Username: "bob", Password: "h", Role: domain.RoleViewer}))
	require.NoError(t, repo.Save(&domain.User{ID: "u-1", Username: "alice", Password: "h", Role: domain.RoleUser}))

	users, err := repo.FindAll()
	require.NoError(t, err)
	require.Len(t, users, 2)
	assert.Equal(t, []string{"alice", "bob"}, []string{users[0].Username, users[1].Username})
}

func TestUserRepository_NotFoundAndInvalid(t *testing.T) {
	db := setupTestDB(t)
	truncateTables(t, db)
	repo := NewUserRepository(db)

	_, err := repo.FindByID("missing")
	assert.ErrorIs(t, err, ports.ErrNotFound)

	assert.ErrorIs(t, repo.Save(nil), ports.ErrInvalidArgument)
	assert.ErrorIs(t, repo.Save(&domain.User{}), ports.ErrInvalidArgument)
}
