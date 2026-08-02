package services

import (
	"context"
	"errors"
	"fmt"

	"portalt/internal/domain"
	"portalt/internal/plugins"
	"portalt/internal/ports"
)

// SyncNativePlugins 把 Registry 中注册的原生插件同步到 plugins 表。
//
// 语义：每个原生插件按 Info() 做 upsert。已存在的记录只更新
// 由代码决定的技术字段（类型/路由/图标/名称），保留管理员在界面上
// 设置过的权限与启用状态——这样「代码升级改菜单结构，界面保权限配置」。
func SyncNativePlugins(ctx context.Context, repo ports.PluginRepository, reg *plugins.Registry) (int, error) {
	count := 0
	for _, p := range reg.All() {
		info := p.Info()
		if err := upsertNativePlugin(ctx, repo, info); err != nil {
			return count, fmt.Errorf("sync native plugin %q: %w", info.ID, err)
		}
		count++
	}
	return count, nil
}

func upsertNativePlugin(ctx context.Context, repo ports.PluginRepository, info domain.Plugin) error {
	existing, err := repo.FindByID(info.ID)
	if errors.Is(err, ports.ErrNotFound) {
		info.Type = domain.PluginTypeNative
		if info.SortOrder == 0 {
			info.SortOrder = 100
		}
		return repo.Save(&info)
	}
	if err != nil {
		return err
	}

	existing.Type = domain.PluginTypeNative
	existing.Name = info.Name
	existing.Icon = info.Icon
	existing.Route = info.Route
	if existing.SortOrder == 0 {
		existing.SortOrder = 100
	}
	return repo.Save(existing)
}
