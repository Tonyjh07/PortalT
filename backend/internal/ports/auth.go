package ports

import (
	"errors"
	"time"

	"portalt/internal/domain"
)

// 认证层错误哨兵。
var (
	// ErrInvalidCredentials 用户名或密码错误
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrInvalidToken 令牌无效或已过期
	ErrInvalidToken = errors.New("invalid or expired token")
)

// AuthenticationProvider 身份认证提供者接口。
// 由 adapters 层实现（local/ldap/sso…），支撑多认证方式切换。
type AuthenticationProvider interface {
	// Authenticate 校验用户名密码，成功返回用户实体
	Authenticate(username, password string) (*domain.User, error)
}

// TokenManager 令牌签发与校验接口。
// 由 adapters 层实现（JWT），供 API 层与中间件使用。
type TokenManager interface {
	// GenerateAccessToken 签发访问令牌（短期）
	GenerateAccessToken(user *domain.User) (string, error)
	// GenerateRefreshToken 签发刷新令牌（长期）
	GenerateRefreshToken(user *domain.User) (string, error)
	// ParseAccessToken 解析并校验访问令牌，失败返回 ErrInvalidToken
	ParseAccessToken(token string) (*domain.User, error)
	// ParseRefreshToken 解析并校验刷新令牌，失败返回 ErrInvalidToken
	ParseRefreshToken(token string) (*domain.User, error)
	// AccessTTL 访问令牌有效期（接口层生成有效期提示用）
	AccessTTL() time.Duration
	// RefreshTTL 刷新令牌有效期
	RefreshTTL() time.Duration
}
