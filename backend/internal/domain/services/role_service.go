package services

import (
	"context"
	"errors"
	"fmt"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

// EnsureDefaultRoles 确保内置角色存在于角色表（启动引导用，幂等）。
// 已存在的角色不会被覆盖——管理员可能已调整过权限矩阵。
func EnsureDefaultRoles(ctx context.Context, roles ports.RoleRepository) error {
	for _, def := range domain.DefaultRoles() {
		if _, err := roles.FindByID(def.ID); err == nil {
			continue
		} else if !errors.Is(err, ports.ErrNotFound) {
			return fmt.Errorf("查询内置角色 %s 失败: %w", def.ID, err)
		}
		if err := roles.Save(def); err != nil {
			return fmt.Errorf("写入内置角色 %s 失败: %w", def.ID, err)
		}
	}
	return nil
}
