package memory

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

// testUser 构造测试用户
func testUser(id, username string, role domain.Role) *domain.User {
	return &domain.User{
		ID:       id,
		Username: username,
		Password: "hash",
		Email:    username + "@lab.local",
		Role:     role,
	}
}

func TestUserRepository_SaveAndFindByID(t *testing.T) {
	repo := NewUserRepository()
	user := testUser("u-1", "alice", domain.RoleAdmin)

	require.NoError(t, repo.Save(user))

	got, err := repo.FindByID("u-1")
	require.NoError(t, err)
	assert.Equal(t, user, got)
}

func TestUserRepository_FindByUsername(t *testing.T) {
	repo := NewUserRepository()
	require.NoError(t, repo.Save(testUser("u-1", "alice", domain.RoleUser)))

	got, err := repo.FindByUsername("alice")
	require.NoError(t, err)
	assert.Equal(t, "u-1", got.ID)

	// 大小写敏感
	_, err = repo.FindByUsername("Alice")
	assert.ErrorIs(t, err, ports.ErrNotFound)

	// 不存在
	_, err = repo.FindByUsername("nobody")
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

func TestUserRepository_FindByID_NotFound(t *testing.T) {
	repo := NewUserRepository()
	got, err := repo.FindByID("missing")
	assert.ErrorIs(t, err, ports.ErrNotFound)
	assert.Nil(t, got)
}

func TestUserRepository_FindAll(t *testing.T) {
	repo := NewUserRepository()
	require.NoError(t, repo.Save(testUser("u-1", "alice", domain.RoleUser)))
	require.NoError(t, repo.Save(testUser("u-2", "bob", domain.RoleViewer)))

	users, err := repo.FindAll()
	require.NoError(t, err)
	assert.Len(t, users, 2)
}

func TestUserRepository_Delete(t *testing.T) {
	repo := NewUserRepository()
	require.NoError(t, repo.Save(testUser("u-1", "alice", domain.RoleUser)))

	require.NoError(t, repo.Delete("u-1"))
	_, err := repo.FindByID("u-1")
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

func TestUserRepository_Delete_NotFound(t *testing.T) {
	repo := NewUserRepository()
	assert.ErrorIs(t, repo.Delete("missing"), ports.ErrNotFound)
}

func TestUserRepository_Save_Invalid(t *testing.T) {
	repo := NewUserRepository()
	assert.ErrorIs(t, repo.Save(nil), ports.ErrInvalidArgument)
	assert.ErrorIs(t, repo.Save(&domain.User{}), ports.ErrInvalidArgument)
}

func TestUserRepository_ConcurrentAccess(t *testing.T) {
	repo := NewUserRepository()
	const n = 100

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := "u-" + string(rune('a'+i%26))
			_ = repo.Save(testUser(id, "user"+id, domain.RoleUser))
			_, _ = repo.FindByUsername("user" + id)
		}(i)
	}
	wg.Wait()

	users, err := repo.FindAll()
	require.NoError(t, err)
	assert.Len(t, users, 26)
}

// 编译期断言：内存仓储实现 ports 接口
var _ ports.UserRepository = (*UserRepository)(nil)
