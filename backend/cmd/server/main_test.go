package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"portalt/internal/adapters/memory"
	"portalt/internal/domain"
	"portalt/internal/pluginhost"
)

// seedFixture 构造一个 CaddyRules 为 rules 的 esxi-admin 插件并执行 seed。
func seedFixture(t *testing.T, rules string) *domain.Plugin {
	t.Helper()
	repo := memory.NewPluginRepository()
	require.NoError(t, repo.Save(&domain.Plugin{
		ID:         "esxi-admin",
		Name:       "ESXi 管理",
		Type:       domain.PluginTypeAccess,
		CaddyRules: rules,
	}))
	require.NoError(t, seedDefaultAccessPlugins(context.Background(), repo))
	got, err := repo.FindByID("esxi-admin")
	require.NoError(t, err)
	return got
}

func TestSeedDefaultAccessPlugins_UpgradeHistoricDefaults(t *testing.T) {
	cases := []struct {
		name   string
		rules  string
		should bool // true = 期望被升级为新默认
	}{
		{"V1 无鉴权旧默认升级", pluginhost.DefaultESXIAdminCaddyRulesV1, true},
		{"V2 缺 ha-nfc 旧默认升级", pluginhost.DefaultESXIAdminCaddyRulesV2, true},
		{"V3 缺 folder/nfc 旧默认升级", pluginhost.DefaultESXIAdminCaddyRulesV3, true},
		{"当前默认保持不变", pluginhost.DefaultESXIAdminCaddyRules, false},
		{"管理员自定义不被覆盖", "handle /custom/* {\n\treverse_proxy 192.168.2.7:80\n}", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := seedFixture(t, tc.rules)
			if tc.should {
				require.Equal(t, pluginhost.DefaultESXIAdminCaddyRules, got.CaddyRules,
					"旧默认应升级为当前默认")
				require.Contains(t, got.CaddyRules, "handle /ha-nfc/*")
				require.Contains(t, got.CaddyRules, "handle /folder*")
				require.Contains(t, got.CaddyRules, "handle /nfc*")
			} else {
				require.Equal(t, tc.rules, got.CaddyRules, "非旧默认形态不应被覆盖")
			}
		})
	}
}

func TestSeedDefaultAccessPlugins_BackfillEmpty(t *testing.T) {
	got := seedFixture(t, "")
	require.Equal(t, pluginhost.DefaultESXIAdminCaddyRules, got.CaddyRules,
		"空规则应回填默认反代规则")
}

func TestSeedDefaultAccessPlugins_CreateNew(t *testing.T) {
	// 记录不存在：seed 走创建分支（不预置记录）
	repo := memory.NewPluginRepository()
	require.NoError(t, seedDefaultAccessPlugins(context.Background(), repo))
	got, err := repo.FindByID("esxi-admin")
	require.NoError(t, err)
	require.Equal(t, "esxi-admin", got.ID)
	require.Equal(t, domain.PluginTypeAccess, domain.NormalizePluginType(got.Type))
	require.Equal(t, domain.PERM_ESXI_ADMIN_USE, got.Permission)
	require.True(t, got.IsActive)
	require.Equal(t, pluginhost.DefaultESXIAdminCaddyRules, got.CaddyRules)
}

func TestSeedDefaultAccessPlugins_Idempotent(t *testing.T) {
	// 升级后二次运行应保持稳定：新默认不再匹配 V1/V2，不会被反复覆盖
	repo := memory.NewPluginRepository()
	require.NoError(t, repo.Save(&domain.Plugin{
		ID:         "esxi-admin",
		Name:       "ESXi 管理",
		Type:       domain.PluginTypeAccess,
		CaddyRules: pluginhost.DefaultESXIAdminCaddyRulesV2,
	}))
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		require.NoError(t, seedDefaultAccessPlugins(ctx, repo))
		got, err := repo.FindByID("esxi-admin")
		require.NoError(t, err)
		require.Equal(t, pluginhost.DefaultESXIAdminCaddyRules, got.CaddyRules)
	}
}
