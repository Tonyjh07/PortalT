// Package auth 提供认证适配器：本地密码认证（bcrypt）与 JWT 令牌管理。
package auth

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

// dummyHash 固定合法的 bcrypt 哈希，用于用户不存在时执行
// 等价的哈希比较，避免通过响应时间差枚举有效用户名。
const dummyHash = "$2a$10$7EqJtq98hPqEX7fNZaFWoO5S1Y9L0h9nYq0qL1Jw5ZzTqL1Jw5Zz."

// LocalProvider 本地密码认证提供者，基于 UserRepository 与 bcrypt。
type LocalProvider struct {
	users ports.UserRepository
}

// NewLocalProvider 创建本地认证提供者。
func NewLocalProvider(users ports.UserRepository) *LocalProvider {
	return &LocalProvider{users: users}
}

// Authenticate 校验用户名与密码，成功返回用户实体。
// 用户名不存在与密码错误均返回 ports.ErrInvalidCredentials。
func (p *LocalProvider) Authenticate(username, password string) (*domain.User, error) {
	if username == "" || password == "" {
		return nil, ports.ErrInvalidCredentials
	}

	user, err := p.users.FindByUsername(username)
	if errors.Is(err, ports.ErrNotFound) {
		// 恒定时间比较，防用户名枚举
		_ = bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
		return nil, ports.ErrInvalidCredentials
	}
	if err != nil {
		return nil, fmt.Errorf("auth: 查询用户失败: %w", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)) != nil {
		return nil, ports.ErrInvalidCredentials
	}
	return user, nil
}

// HashPassword 生成 bcrypt 密码哈希（默认成本因子）。
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("auth: 密码哈希失败: %w", err)
	}
	return string(b), nil
}

// EnsureAdminUser 确保管理员账号存在（启动引导用）。
// 已存在则跳过；不存在则以给定角色为 admin 创建。
func EnsureAdminUser(ctx context.Context, users ports.UserRepository, username, password string) error {
	if username == "" || password == "" {
		return fmt.Errorf("auth: 管理员账号与密码不能为空")
	}

	_, err := users.FindByUsername(username)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ports.ErrNotFound) {
		return fmt.Errorf("auth: 查询管理员失败: %w", err)
	}

	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	admin := &domain.User{
		ID:       newID(),
		Username: username,
		Password: hash,
		Role:     domain.RoleAdmin,
	}
	if err := users.Save(admin); err != nil {
		return fmt.Errorf("auth: 创建管理员失败: %w", err)
	}
	return nil
}
