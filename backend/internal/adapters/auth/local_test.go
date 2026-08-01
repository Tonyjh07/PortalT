package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/adapters/memory"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

// setupLocal 构建带测试用户的本地认证提供者。
func setupLocal(t *testing.T) (*LocalProvider, *memory.UserRepository) {
	t.Helper()
	repo := memory.NewUserRepository()

	hash, err := HashPassword("s3cret!")
	require.NoError(t, err)
	require.NoError(t, repo.Save(&domain.User{
		ID:       "u-1",
		Username: "alice",
		Password: hash,
		Role:     domain.RoleAdmin,
	}))
	return NewLocalProvider(repo), repo
}

func TestLocalProvider_Authenticate_Success(t *testing.T) {
	p, _ := setupLocal(t)

	user, err := p.Authenticate("alice", "s3cret!")
	require.NoError(t, err)
	assert.Equal(t, "alice", user.Username)
	assert.Equal(t, domain.RoleAdmin, user.Role)
}

func TestLocalProvider_Authenticate_WrongPassword(t *testing.T) {
	p, _ := setupLocal(t)

	_, err := p.Authenticate("alice", "wrong-password")
	assert.ErrorIs(t, err, ports.ErrInvalidCredentials)
}

func TestLocalProvider_Authenticate_UnknownUser(t *testing.T) {
	p, _ := setupLocal(t)

	_, err := p.Authenticate("nobody", "s3cret!")
	assert.ErrorIs(t, err, ports.ErrInvalidCredentials)
}

func TestLocalProvider_Authenticate_EmptyInput(t *testing.T) {
	p, _ := setupLocal(t)

	_, err := p.Authenticate("", "")
	assert.ErrorIs(t, err, ports.ErrInvalidCredentials)
	_, err = p.Authenticate("alice", "")
	assert.ErrorIs(t, err, ports.ErrInvalidCredentials)
}

func TestLocalProvider_Authenticate_RepoErrorPropagates(t *testing.T) {
	repo := &failingRepo{err: errors.New("db down")}
	p := NewLocalProvider(repo)

	_, err := p.Authenticate("alice", "s3cret!")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ports.ErrInvalidCredentials)
	assert.ErrorContains(t, err, "db down")
}

// failingRepo 模拟仓储故障，验证错误正确传播。
type failingRepo struct{ err error }

func (r *failingRepo) Save(*domain.User) error                        { return r.err }
func (r *failingRepo) FindByID(string) (*domain.User, error)          { return nil, r.err }
func (r *failingRepo) FindByUsername(string) (*domain.User, error)    { return nil, r.err }
func (r *failingRepo) FindAll() ([]*domain.User, error)               { return nil, r.err }
func (r *failingRepo) Delete(string) error                            { return r.err }

func TestHashPassword(t *testing.T) {
	hash, err := HashPassword("hello")
	require.NoError(t, err)
	assert.NotEqual(t, "hello", hash)

	// 相同密码每次哈希应不同（随机盐）
	hash2, err := HashPassword("hello")
	require.NoError(t, err)
	assert.NotEqual(t, hash, hash2)
}

func TestEnsureAdminUser(t *testing.T) {
	repo := memory.NewUserRepository()
	ctx := context.Background()

	require.NoError(t, EnsureAdminUser(ctx, repo, "admin", "admin123"))

	user, err := repo.FindByUsername("admin")
	require.NoError(t, err)
	assert.Equal(t, domain.RoleAdmin, user.Role)
	assert.NotEmpty(t, user.ID)
	assert.NotEqual(t, "admin123", user.Password, "密码应已哈希")

	// 再次调用应幂等
	require.NoError(t, EnsureAdminUser(ctx, repo, "admin", "admin123"))
	users, err := repo.FindAll()
	require.NoError(t, err)
	assert.Len(t, users, 1, "不应重复创建管理员")
}

func TestEnsureAdminUser_EmptyInput(t *testing.T) {
	repo := memory.NewUserRepository()
	assert.Error(t, EnsureAdminUser(context.Background(), repo, "", ""))
}
