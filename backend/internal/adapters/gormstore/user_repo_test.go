package gormstore

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

func TestUserRepository_Crud(t *testing.T) {
	db := newTestDB(t)
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

	require.NoError(t, repo.Delete("u-1"))
	_, err = repo.FindByID("u-1")
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

func TestUserRepository_FindByUsername(t *testing.T) {
	db := newTestDB(t)
	repo := NewUserRepository(db)
	require.NoError(t, repo.Save(&domain.User{ID: "u-1", Username: "bob", Password: "h", Role: domain.RoleViewer}))

	got, err := repo.FindByUsername("bob")
	require.NoError(t, err)
	assert.Equal(t, "u-1", got.ID)

	_, err = repo.FindByUsername("nobody")
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

func TestUserRepository_UsernameUnique(t *testing.T) {
	db := newTestDB(t)
	repo := NewUserRepository(db)
	require.NoError(t, repo.Save(&domain.User{ID: "u-1", Username: "dup", Password: "h", Role: domain.RoleUser}))

	err := repo.Save(&domain.User{ID: "u-2", Username: "dup", Password: "h", Role: domain.RoleUser})
	assert.Error(t, err)
}

func TestUserRepository_FindAll_Ordered(t *testing.T) {
	db := newTestDB(t)
	repo := NewUserRepository(db)
	require.NoError(t, repo.Save(&domain.User{ID: "u-2", Username: "bob", Password: "h", Role: domain.RoleViewer}))
	require.NoError(t, repo.Save(&domain.User{ID: "u-1", Username: "alice", Password: "h", Role: domain.RoleUser}))

	users, err := repo.FindAll()
	require.NoError(t, err)
	require.Len(t, users, 2)
	assert.Equal(t, "alice", users[0].Username)
	assert.Equal(t, "bob", users[1].Username)
}

func TestUserRepository_Errors(t *testing.T) {
	db := newTestDB(t)
	repo := NewUserRepository(db)

	_, err := repo.FindByID("missing")
	assert.ErrorIs(t, err, ports.ErrNotFound)
	assert.ErrorIs(t, repo.Save(nil), ports.ErrInvalidArgument)
	assert.ErrorIs(t, repo.Save(&domain.User{}), ports.ErrInvalidArgument)
}

func TestUserRepository_ConcurrentReadWrite(t *testing.T) {
	db := newTestDB(t)
	repo := NewUserRepository(db)
	require.NoError(t, repo.Save(&domain.User{ID: "u-1", Username: "a", Password: "h", Role: domain.RoleUser}))

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if i%2 == 0 {
				_, _ = repo.FindAll()
			} else {
				_ = repo.Save(&domain.User{ID: "u-1", Username: "a", Password: "h", Role: domain.RoleUser})
			}
		}(i)
	}
	wg.Wait()
}
