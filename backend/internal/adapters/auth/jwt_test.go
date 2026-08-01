package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

func testUser() *domain.User {
	return &domain.User{ID: "u-1", Username: "alice", Role: domain.RoleAdmin}
}

func TestJWT_RoundTrip(t *testing.T) {
	m := NewJWTManager("test-secret", 5*time.Minute, 24*time.Hour)

	access, err := m.GenerateAccessToken(testUser())
	require.NoError(t, err)
	user, err := m.ParseAccessToken(access)
	require.NoError(t, err)
	assert.Equal(t, "u-1", user.ID)
	assert.Equal(t, "alice", user.Username)
	assert.Equal(t, domain.RoleAdmin, user.Role)
}

func TestJWT_RefreshTokenRoundTrip(t *testing.T) {
	m := NewJWTManager("test-secret", 5*time.Minute, 24*time.Hour)

	refresh, err := m.GenerateRefreshToken(testUser())
	require.NoError(t, err)
	user, err := m.ParseRefreshToken(refresh)
	require.NoError(t, err)
	assert.Equal(t, "alice", user.Username)
}

func TestJWT_TypeMismatch(t *testing.T) {
	m := NewJWTManager("test-secret", 5*time.Minute, 24*time.Hour)

	access, err := m.GenerateAccessToken(testUser())
	require.NoError(t, err)
	refresh, err := m.GenerateRefreshToken(testUser())
	require.NoError(t, err)

	// 访问令牌不能当刷新令牌用，反之亦然
	_, err = m.ParseRefreshToken(access)
	assert.ErrorIs(t, err, ports.ErrInvalidToken)
	_, err = m.ParseAccessToken(refresh)
	assert.ErrorIs(t, err, ports.ErrInvalidToken)
}

func TestJWT_ExpiredToken(t *testing.T) {
	m := NewJWTManager("test-secret", 5*time.Minute, 24*time.Hour)

	// 手工构造已过期令牌（NewJWTManager 会拒绝非正 TTL）
	claims := jwtClaims{
		Username: "alice",
		Role:     domain.RoleAdmin,
		Type:     TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "u-1",
			Issuer:    "portalt",
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Minute)),
			NotBefore: jwt.NewNumericDate(time.Now().Add(-2 * time.Minute)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Minute)),
		},
	}
	expired, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("test-secret"))
	require.NoError(t, err)

	_, err = m.ParseAccessToken(expired)
	assert.ErrorIs(t, err, ports.ErrInvalidToken)
}

func TestJWT_WrongSecret(t *testing.T) {
	signer := NewJWTManager("secret-a", 5*time.Minute, 24*time.Hour)
	verifier := NewJWTManager("secret-b", 5*time.Minute, 24*time.Hour)

	access, err := signer.GenerateAccessToken(testUser())
	require.NoError(t, err)
	_, err = verifier.ParseAccessToken(access)
	assert.ErrorIs(t, err, ports.ErrInvalidToken)
}

func TestJWT_TamperedToken(t *testing.T) {
	m := NewJWTManager("test-secret", 5*time.Minute, 24*time.Hour)

	access, err := m.GenerateAccessToken(testUser())
	require.NoError(t, err)
	tampered := access[:len(access)-2] + "xx"

	_, err = m.ParseAccessToken(tampered)
	assert.ErrorIs(t, err, ports.ErrInvalidToken)
}

func TestJWT_GarbageToken(t *testing.T) {
	m := NewJWTManager("test-secret", 5*time.Minute, 24*time.Hour)
	_, err := m.ParseAccessToken("not-a-jwt")
	assert.ErrorIs(t, err, ports.ErrInvalidToken)
}

func TestJWT_Defaults(t *testing.T) {
	m := NewJWTManager("", 0, 0)
	assert.Equal(t, DefaultAccessTTL, m.AccessTTL())
	assert.Equal(t, DefaultRefreshTTL, m.RefreshTTL())
}

func TestJWT_GenerateNilUser(t *testing.T) {
	m := NewJWTManager("test-secret", 5*time.Minute, 24*time.Hour)
	_, err := m.GenerateAccessToken(nil)
	assert.Error(t, err)
}
