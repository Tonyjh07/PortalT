package pluginhost

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/adapters/memory"
	"portalt/internal/domain"
	"portalt/internal/ports"
)

// 3.1 单元测试：不 spawn 真实进程，聚焦 manifest 校验、DB 同步、
// 目录缺失标记与禁用态管理器的幂等行为。真实 spawn 见
// manager_integration_test.go（build tag: integration）。

// writeFakePlugin 在 dir 下构造一个最小可识别插件目录（manifest + 占位可执行文件）。
func writeFakePlugin(t *testing.T, dir, id string, manifest string) string {
	t.Helper()
	pdir := filepath.Join(dir, id)
	require.NoError(t, os.MkdirAll(pdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pdir, "manifest.json"), []byte(manifest), 0o644))
	bin := filepath.Join(pdir, "plugin")
	if err := os.WriteFile(bin, []byte("dummy"), 0o755); err != nil {
		// Windows 下不可执行文件也允许（本测试不 spawn）
		require.NoError(t, os.WriteFile(bin, []byte("dummy"), 0o644))
	}
	return pdir
}

const helloManifest = `{
	"id": "hello",
	"name": "Hello 示例插件",
	"icon": "mdi:hand-wave",
	"route": "/hello",
	"sort_order": 200,
	"permission": "",
	"health_interval_seconds": 5
}`

func TestManager_Disabled(t *testing.T) {
	m := NewManager("", memory.NewPluginRepository(), "test")
	assert.True(t, m.Disabled())
	assert.False(t, m.Enabled())
	require.NoError(t, m.Start(t.Context()))
	m.Shutdown(t.Context())
	assert.Equal(t, "", m.Status("hello"))
	assert.Equal(t, "", m.HTTPAddress("hello"))
}

func TestManager_Load_UpsertNewDefaultDisabled(t *testing.T) {
	dir := t.TempDir()
	writeFakePlugin(t, dir, "hello", helloManifest)
	repo := memory.NewPluginRepository()
	m := NewManager(dir, repo, "test")

	require.NoError(t, m.Load(t.Context()))
	p, err := repo.FindByID("hello")
	require.NoError(t, err)
	assert.Equal(t, domain.PluginTypeNative, p.Type)
	assert.False(t, p.IsActive, "新插件默认禁用，需管理员启用")
	assert.Equal(t, StatusStopped, p.Status)
	assert.Contains(t, p.ManifestJSON, `"id":"hello"`)
	// 未启用 → 不 spawn
	assert.Equal(t, "", m.HTTPAddress("hello"))
}

func TestManager_Load_ManifestIDMismatch(t *testing.T) {
	dir := t.TempDir()
	writeFakePlugin(t, dir, "hello", `{"id":"other","name":"x","route":"/x"}`)
	repo := memory.NewPluginRepository()
	m := NewManager(dir, repo, "test")
	require.NoError(t, m.Load(t.Context()))
	_, err := repo.FindByID("hello")
	assert.ErrorIs(t, err, ports.ErrNotFound, "manifest.id 与目录名不一致时不入库")
}

func TestManager_Load_NotInstalledSkipped(t *testing.T) {
	dir := t.TempDir()
	// 只有 manifest 没有可执行文件
	pdir := filepath.Join(dir, "hello")
	require.NoError(t, os.MkdirAll(pdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pdir, "manifest.json"), []byte(helloManifest), 0o644))
	repo := memory.NewPluginRepository()
	m := NewManager(dir, repo, "test")
	require.NoError(t, m.Load(t.Context()))
	_, err := repo.FindByID("hello")
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

func TestManager_Load_PreservesAdminConfig(t *testing.T) {
	dir := t.TempDir()
	writeFakePlugin(t, dir, "hello", helloManifest)
	repo := memory.NewPluginRepository()
	require.NoError(t, repo.Save(&domain.Plugin{
		ID: "hello", Name: "旧名", Route: "/hello", Type: domain.PluginTypeNative,
		Permission: "vm:view",
	}))
	m := NewManager(dir, repo, "test")
	require.NoError(t, m.Load(t.Context()))
	p, err := repo.FindByID("hello")
	require.NoError(t, err)
	assert.Equal(t, "Hello 示例插件", p.Name, "manifest 字段更新")
	assert.False(t, p.IsActive, "管理员未启用则不 spawn")
	assert.Equal(t, "vm:view", p.Permission, "管理员权限配置保留")
}

func TestManager_Load_MarksMissing(t *testing.T) {
	repo := memory.NewPluginRepository()
	require.NoError(t, repo.Save(&domain.Plugin{
		ID: "gone", Name: "已删", Route: "/gone", Type: domain.PluginTypeNative,
	}))
	m := NewManager(t.TempDir(), repo, "test")
	require.NoError(t, m.Load(t.Context()))
	p, err := repo.FindByID("gone")
	require.NoError(t, err)
	assert.Equal(t, StatusMissing, p.Status, "目录被删除 → 标记 missing，记录保留")
}

func TestManager_Upsert_NonNativeIDConflict(t *testing.T) {
	dir := t.TempDir()
	writeFakePlugin(t, dir, "esxi", `{"id":"esxi","name":"x","route":"/x"}`)
	repo := memory.NewPluginRepository()
	require.NoError(t, repo.Save(&domain.Plugin{
		ID: "esxi", Name: "ESXi", Route: "/esxi-admin", Type: domain.PluginTypeAccess,
	}))
	m := NewManager(dir, repo, "test")
	// access 插件占用同名 ID → 同步报错被跳过（Load 对单插件失败不致命），记录不被覆盖
	require.NoError(t, m.Load(t.Context()))
	p, err := repo.FindByID("esxi")
	require.NoError(t, err)
	assert.Equal(t, domain.PluginTypeAccess, p.Type)
	assert.Equal(t, "ESXi", p.Name)
}

func TestManager_Status_Unknown(t *testing.T) {
	m := NewManager(t.TempDir(), memory.NewPluginRepository(), "test")
	assert.Equal(t, "", m.Status("ghost"))
	assert.Equal(t, "", m.HTTPAddress("ghost"))
}

func TestManager_HTTPAddress_Stopped(t *testing.T) {
	dir := t.TempDir()
	writeFakePlugin(t, dir, "hello", helloManifest)
	repo := memory.NewPluginRepository()
	require.NoError(t, repo.Save(&domain.Plugin{
		ID: "hello", Name: "Hello", Route: "/hello", Type: domain.PluginTypeNative,
		IsActive: false, Status: StatusStopped,
	}))
	m := NewManager(dir, repo, "test")
	require.NoError(t, m.Load(t.Context()))
	assert.Equal(t, "", m.HTTPAddress("hello"), "未运行 → 无地址")
}

func TestFreePort(t *testing.T) {
	ports := make(map[int]bool, 8)
	for i := 0; i < 8; i++ {
		p, err := freePort()
		require.NoError(t, err)
		assert.Positive(t, p)
		ports[p] = true
	}
	assert.GreaterOrEqual(t, len(ports), 2, "应分配过空闲端口（少量重复可容忍）")
}

// TestStopProc_NonRunningBlocksRespawn B1 回归：崩溃退避重启期间调用 stopProc，
// 必须置 stopping 阻止退避线程复活进程（否则停机/停用后残留孤儿进程）。
func TestStopProc_NonRunningBlocksRespawn(t *testing.T) {
	dir := t.TempDir()
	writeFakePlugin(t, dir, "hello", helloManifest)
	repo := memory.NewPluginRepository()
	require.NoError(t, repo.Save(&domain.Plugin{
		ID: "hello", Name: "Hello", Route: "/hello", Type: domain.PluginTypeNative,
		IsActive: true, Status: StatusError, // 崩溃后 error 态
	}))
	m := NewManager(dir, repo, "test")
	require.NoError(t, m.Load(t.Context()))
	proc, err := m.procFor("hello")
	require.NoError(t, err)

	// 模拟：进程已崩溃（非运行），stopProc 仍须置 stopping
	m.stopProc(t.Context(), "hello", "shutdown")
	proc.mu.Lock()
	stopping := proc.stopping
	proc.mu.Unlock()
	assert.True(t, stopping, "非运行态 stopProc 也必须置 stopping，防退避重启复活")
	assert.Equal(t, StatusStopped, proc.Status())

	// spawn 在 stopping 下必须拒绝拉起
	require.NoError(t, m.spawn(t.Context(), proc), "stopping 下 spawn 应静默拒绝")
	proc.mu.Lock()
	spawning := proc.spawning
	proc.mu.Unlock()
	assert.False(t, spawning, "spawn 不得在 stopping 下启动进程")
}

// TestSpawn_ConcurrentSerialized R6 回归：并发 spawn 同一插件只允许一个真正拉起。
func TestSpawn_ConcurrentSerialized(t *testing.T) {
	dir := t.TempDir()
	writeFakePlugin(t, dir, "hello", helloManifest)
	repo := memory.NewPluginRepository()
	require.NoError(t, repo.Save(&domain.Plugin{
		ID: "hello", Name: "Hello", Route: "/hello", Type: domain.PluginTypeNative,
		IsActive: true,
	}))
	m := NewManager(dir, repo, "test")
	require.NoError(t, m.Load(t.Context()))
	proc, err := m.procFor("hello")
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.spawn(t.Context(), proc)
		}()
	}
	wg.Wait()
	proc.mu.Lock()
	defer proc.mu.Unlock()
	assert.NotEqual(t, StatusRunning, proc.status,
		"占位可执行文件无法启动：并发 spawn 不允许任何一次把进程标为 running（防双进程）")
	assert.False(t, proc.spawning, "spawn 结束后 spawning 标志必须复位")
}
