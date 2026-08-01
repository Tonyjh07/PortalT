package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

// 令牌类型声明。
const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

// defaultTTL 默认有效期：访问令牌 15 分钟，刷新令牌 7 天。
const (
	DefaultAccessTTL  = 15 * time.Minute
	DefaultRefreshTTL = 7 * 24 * time.Hour
)

// jwtClaims JWT 载荷：标准注册声明 + 用户信息 + 令牌类型。
type jwtClaims struct {
	Username string      `json:"username"`
	Role     domain.Role `json:"role"`
	Type     string      `json:"type"`
	jwt.RegisteredClaims
}

// JWTManager 基于 HS256 的令牌管理器。
type JWTManager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

// NewJWTManager 创建 JWT 管理器。
// secret 为空时使用默认开发密钥并打警告（生产必须显式配置）。
func NewJWTManager(secret string, accessTTL, refreshTTL time.Duration) *JWTManager {
	if secret == "" {
		secret = "dev-only-secret-do-not-use-in-production"
	}
	if accessTTL <= 0 {
		accessTTL = DefaultAccessTTL
	}
	if refreshTTL <= 0 {
		refreshTTL = DefaultRefreshTTL
	}
	return &JWTManager{secret: []byte(secret), accessTTL: accessTTL, refreshTTL: refreshTTL}
}

// GenerateAccessToken 签发访问令牌。
func (m *JWTManager) GenerateAccessToken(user *domain.User) (string, error) {
	return m.generate(user, TokenTypeAccess, m.accessTTL)
}

// GenerateRefreshToken 签发刷新令牌。
func (m *JWTManager) GenerateRefreshToken(user *domain.User) (string, error) {
	return m.generate(user, TokenTypeRefresh, m.refreshTTL)
}

// ParseAccessToken 解析校验访问令牌。
func (m *JWTManager) ParseAccessToken(token string) (*domain.User, error) {
	return m.parse(token, TokenTypeAccess)
}

// ParseRefreshToken 解析校验刷新令牌。
func (m *JWTManager) ParseRefreshToken(token string) (*domain.User, error) {
	return m.parse(token, TokenTypeRefresh)
}

// AccessTTL 返回访问令牌有效期。
func (m *JWTManager) AccessTTL() time.Duration { return m.accessTTL }

// RefreshTTL 返回刷新令牌有效期。
func (m *JWTManager) RefreshTTL() time.Duration { return m.refreshTTL }

// generate 签发指定类型令牌。
func (m *JWTManager) generate(user *domain.User, typ string, ttl time.Duration) (string, error) {
	if user == nil {
		return "", fmt.Errorf("jwt: 用户不能为空")
	}
	now := time.Now()
	claims := jwtClaims{
		Username: user.Username,
		Role:     user.Role,
		Type:     typ,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID,
			Issuer:    "portalt",
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// parse 解析校验令牌并校验类型。
func (m *JWTManager) parse(tokenStr, wantType string) (*domain.User, error) {
	claims := &jwtClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("jwt: 非预期签名算法 %v", t.Method)
		}
		return m.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, ports.ErrInvalidToken
	}
	if claims.Type != wantType {
		return nil, ports.ErrInvalidToken
	}
	return &domain.User{
		ID:       claims.Subject,
		Username: claims.Username,
		Role:     claims.Role,
	}, nil
}
