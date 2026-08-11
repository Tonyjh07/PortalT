package pluginhost

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/domain"
)

// writeFakeCaddy 在 dir 下生成可控退出码的假 caddy 可执行文件
// （退出码取环境变量 PORTALT_FAKE_CADDY_EXIT，缺省 0）。
func writeFakeCaddy(t *testing.T, dir string) {
	t.Helper()
	name := "caddy"
	var body string
	if runtime.GOOS == "windows" {
		name = "caddy.cmd"
		body = "@echo off\r\nif \"%PORTALT_FAKE_CADDY_EXIT%\"==\"\" (exit /b 0) else (exit /b %PORTALT_FAKE_CADDY_EXIT%)\r\n"
	} else {
		body = "#!/bin/sh\nexit ${PORTALT_FAKE_CADDY_EXIT:-0}\n"
	}
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	if runtime.GOOS != "windows" {
		require.NoError(t, os.Chmod(path, 0o755))
	}
}

func TestCaddyManager_ApplyValidatesCaddy(t *testing.T) {
	// PATH 注入假 caddy：校验失败不落盘
	binDir := t.TempDir()
	writeFakeCaddy(t, binDir)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	dir := t.TempDir()
	m := NewCaddyManager(dir, "")

	// 校验通过 → 落盘
	t.Setenv("PORTALT_FAKE_CADDY_EXIT", "0")
	require.NoError(t, m.Apply("demo", "handle /x/* { respond 200 }"))
	_, err := os.Stat(filepath.Join(dir, "demo.caddy"))
	require.NoError(t, err)

	// 校验失败 → 报错且不落盘
	t.Setenv("PORTALT_FAKE_CADDY_EXIT", "1")
	err = m.Apply("bad", "handle /x/* { syntax error")
	require.Error(t, err)
	_, err = os.Stat(filepath.Join(dir, "bad.caddy"))
	assert.True(t, os.IsNotExist(err))
}

func TestCaddyManager_ApplySkipsValidateWhenEnvPlaceholder(t *testing.T) {
	// 片段含 {env.*} 占位符 → 跳过校验（即使假 caddy 退出 1 也照常落盘，避免误报）
	binDir := t.TempDir()
	writeFakeCaddy(t, binDir)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PORTALT_FAKE_CADDY_EXIT", "1")

	dir := t.TempDir()
	m := NewCaddyManager(dir, "")
	require.NoError(t, m.Apply("demo", "handle /x/* { reverse_proxy {env.ESXI_UPSTREAM}:443 }"))
	_, err := os.Stat(filepath.Join(dir, "demo.caddy"))
	require.NoError(t, err)
}

func TestCaddyManager_ApplySkipsValidateWithoutCaddy(t *testing.T) {
	// PATH 中无 caddy（dev 环境）→ 跳过校验直接落盘
	t.Setenv("PATH", t.TempDir())
	dir := t.TempDir()
	m := NewCaddyManager(dir, "")
	require.NoError(t, m.Apply("demo", "handle /x/* {}"))
	_, err := os.Stat(filepath.Join(dir, "demo.caddy"))
	require.NoError(t, err)
}

func TestCaddyManager_ApplyWritesFile(t *testing.T) {
	dir := t.TempDir()
	m := NewCaddyManager(dir, "")
	require.NoError(t, m.Apply("demo", "handle /x/* { respond 200 }"))
	data, err := os.ReadFile(filepath.Join(dir, "demo.caddy"))
	require.NoError(t, err)
	assert.Equal(t, "handle /x/* { respond 200 }", string(data))
}

func TestCaddyManager_ApplyEmptyRemoves(t *testing.T) {
	dir := t.TempDir()
	m := NewCaddyManager(dir, "")
	require.NoError(t, m.Apply("demo", "handle /x/* { respond 200 }"))
	require.NoError(t, m.Apply("demo", ""))
	_, err := os.Stat(filepath.Join(dir, "demo.caddy"))
	assert.True(t, os.IsNotExist(err))
}

func TestCaddyManager_Remove(t *testing.T) {
	dir := t.TempDir()
	m := NewCaddyManager(dir, "")
	require.NoError(t, m.Apply("demo", "handle /x/* {}"))
	require.NoError(t, m.Remove("demo"))
	_, err := os.Stat(filepath.Join(dir, "demo.caddy"))
	assert.True(t, os.IsNotExist(err))
	// 再次移除幂等
	require.NoError(t, m.Remove("demo"))
}

func TestCaddyManager_SyncAllAligns(t *testing.T) {
	dir := t.TempDir()
	// 预先放置孤儿文件（旧插件残留），SyncAll 应清理
	require.NoError(t, os.WriteFile(filepath.Join(dir, "orphan.caddy"), []byte("x"), 0o644))
	m := NewCaddyManager(dir, "")
	plugins := []*domain.Plugin{
		{ID: "a", Type: domain.PluginTypeAccess, IsActive: true, CaddyRules: "handle /a/* {}"},
		{ID: "b", Type: domain.PluginTypeAccess, IsActive: true, CaddyRules: ""},
		{ID: "c", Type: domain.PluginTypeNative, IsActive: true, CaddyRules: "handle /c/* {}"},
		{ID: "d", Type: domain.PluginTypeAccess, IsActive: false, CaddyRules: "handle /d/* {}"},
	}
	require.NoError(t, m.SyncAll(plugins))

	cases := map[string]bool{
		"a.caddy":     true,
		"b.caddy":     false,
		"c.caddy":     false,
		"d.caddy":     false,
		"orphan.caddy": false,
	}
	for name, want := range cases {
		_, err := os.Stat(filepath.Join(dir, name))
		assert.Equal(t, want, err == nil, "文件 %s", name)
	}
}

func TestCaddyManager_SyncAllPartialFailureKeepsOldFile(t *testing.T) {
	// 校验依赖 caddy 二进制：不可用时跳过（与 TestCaddyManager_Reload 的 sh 跳过同理）
	if _, err := exec.LookPath("caddy"); err != nil {
		t.Skip("caddy 不可用，跳过 SyncAll 部分失败测试")
	}
	// 插件 e 校验失败：仅跳过自身并保留其旧规则文件，其余插件正常对齐，
	// 成功写入/清理仍走 Reload（reloadCmd 为空时 Reload 为 no-op，不依赖 sh）
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "e.caddy"), []byte("handle /e/* {}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "orphan.caddy"), []byte("x"), 0o644))
	m := NewCaddyManager(dir, "")
	plugins := []*domain.Plugin{
		{ID: "a", Type: domain.PluginTypeAccess, IsActive: true, CaddyRules: "handle /a/* {}"},
		{ID: "e", Type: domain.PluginTypeAccess, IsActive: true, CaddyRules: "bad-rules" /* 校验失败 */},
	}
	err := m.SyncAll(plugins)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "校验/落盘失败")

	// e 旧规则文件保留（未被清理循环删除），a 已写入，孤儿文件已清理
	_, err = os.Stat(filepath.Join(dir, "e.caddy"))
	assert.NoError(t, err, "校验失败的插件应保留旧规则文件")
	_, err = os.Stat(filepath.Join(dir, "a.caddy"))
	assert.NoError(t, err, "成功插件应已落盘")
	_, err = os.Stat(filepath.Join(dir, "orphan.caddy"))
	assert.True(t, os.IsNotExist(err), "孤儿文件应被清理")
}

func TestCaddyManager_Reload(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh 不可用，跳过 reload 命令测试")
	}
	dir := t.TempDir()
	// 成功路径
	require.NoError(t, NewCaddyManager(dir, "true").Reload())
	// 失败路径 → ErrReloadFailed
	err := NewCaddyManager(dir, "false").Reload()
	require.Error(t, err)
	assert.True(t, IsReloadFailed(err))
}

func TestCaddyManager_ApplyWriteOnly(t *testing.T) {
	// Apply 仅落盘，不触发 reload：reload 命令配置为必然失败也不报错
	dir := t.TempDir()
	m := NewCaddyManager(dir, "false")
	require.NoError(t, m.Apply("demo", "handle /x/* {}"))
	_, err := os.Stat(filepath.Join(dir, "demo.caddy"))
	assert.NoError(t, err)
}

func TestCaddyManager_UnconfiguredReloadNoOp(t *testing.T) {
	// reloadCmd 为空 → 不执行命令，不报错
	require.NoError(t, NewCaddyManager(t.TempDir(), "").Reload())
	require.NoError(t, (*CaddyManager)(nil).Apply("a", "b"))
	require.NoError(t, (*CaddyManager)(nil).Remove("a"))
}

func TestCaddyManager_EmptyRulesDirNoOp(t *testing.T) {
	// 未配置规则目录（本地 dev 无 Caddy）→ 不落盘不报错
	m := NewCaddyManager("", "")
	require.False(t, m.Enabled(), "未配置规则目录时 Enabled 应为 false")
	require.False(t, m.ReloadEnabled(), "未配置 reload 命令时 ReloadEnabled 应为 false")
	require.NoError(t, m.Apply("demo", "handle /x/* {}"))
	require.NoError(t, m.Reload())
	require.NoError(t, m.Remove("demo"))
	require.NoError(t, m.SyncAll([]*domain.Plugin{{ID: "a", Type: domain.PluginTypeAccess, IsActive: true, CaddyRules: "x"}}))
}

func TestCaddyManager_ReloadEnabledFlag(t *testing.T) {
	// 规则目录与 reload 命令独立配置：目录非空但命令为空时仅落盘能力可用
	m := NewCaddyManager(t.TempDir(), "")
	require.True(t, m.Enabled())
	require.False(t, m.ReloadEnabled())
	m2 := NewCaddyManager(t.TempDir(), "systemctl reload caddy")
	require.True(t, m2.Enabled())
	require.True(t, m2.ReloadEnabled())
	require.False(t, (*CaddyManager)(nil).Enabled())
	require.False(t, (*CaddyManager)(nil).ReloadEnabled())
}

func TestIsReloadFailed_Nil(t *testing.T) {
	assert.False(t, IsReloadFailed(nil))
}

func TestDefaultESXIAdminCaddyRules(t *testing.T) {
	assert.Contains(t, DefaultESXIAdminCaddyRules, "handle /esxi/*")
	assert.Contains(t, DefaultESXIAdminCaddyRules, "handle /ui/*")
	assert.Contains(t, DefaultESXIAdminCaddyRules, "{env.ESXI_UPSTREAM}")
	assert.Contains(t, DefaultESXIAdminCaddyRules, "handle /ticket*")
	// 每个 ESXi handle 都须带鉴权闸口回调
	assert.Contains(t, DefaultESXIAdminCaddyRules, "forward_auth 127.0.0.1:8080")
	assert.Contains(t, DefaultESXIAdminCaddyRules, "/api/v1/auth/gate?perm=esxi-admin:use")
	// /ha-nfc NFC 端点：虚拟机导出/下载时 HEAD 测 size，必须代理
	assert.Contains(t, DefaultESXIAdminCaddyRules, "handle /ha-nfc/*")
	// 版本迁移：新默认规则须与旧无鉴权版默认不同
	assert.NotEqual(t, DefaultESXIAdminCaddyRules, DefaultESXIAdminCaddyRulesV1)
	assert.NotContains(t, DefaultESXIAdminCaddyRulesV1, "forward_auth")
	// 缺 /ha-nfc 的旧默认（V2）须与新默认不同
	assert.NotEqual(t, DefaultESXIAdminCaddyRules, DefaultESXIAdminCaddyRulesV2)
	assert.NotContains(t, DefaultESXIAdminCaddyRulesV2, "handle /ha-nfc/*")
}
