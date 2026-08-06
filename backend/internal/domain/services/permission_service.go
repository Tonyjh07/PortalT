package services

import (
	"context"
	"fmt"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

// EnsureDefaultPermissions 确保默认权限字典存在于 permissions 表（启动引导用，幂等）。
// 以代码里的 AllPermissions() 为单一事实来源；已存在的权限不覆盖（保留自定义描述）。
func EnsureDefaultPermissions(ctx context.Context, perms ports.PermissionRepository) error {
	if err := perms.EnsureDefault(domain.AllPermissions()); err != nil {
		return fmt.Errorf("写入默认权限字典失败: %w", err)
	}
	return nil
}
