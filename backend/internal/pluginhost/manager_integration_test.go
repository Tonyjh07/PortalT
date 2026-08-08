//go:build integration

package pluginhost

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/adapters/memory"
)

// 3.1 集成测试：真实构建并 spawn 示例插件 backend/plugins/examples/hello，
// 覆盖 加载→启用→运行→反代→停用→重启 全生命周期。
// 运行方式：go test -tags integration ./internal/pluginhost/...（构建插件耗时属正常）。

// buildHelloPlugin 把示例插件编译到 PLUGINS_DIR/hello/plugin(.exe)，返回插件目录。
func buildHelloPlugin(t *testing.T, pluginsDir string) string {
	t.Helper()
	goBin := filepath.Join(runtime.GOROOT(), "bin", "go")
	if runtime.GOOS == "windows" {
		goBin += ".exe"
	}
	binName := "plugin"
	if runtime.GOOS == "windows" {
		binName = "plugin.exe"
	}
	out := filepath.Join(pluginsDir, "hello", binName)
	require.NoError(t, os.MkdirAll(filepath.Dir(out), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pluginsDir, "hello", "manifest.json"), []byte(helloManifest), 0o644))

	cmd := exec.Command(goBin, "build", "-o", out, "portalt/plugins/examples/hello/cmd/hello")
	cmd.Dir = pluginModuleRoot(t)
	output, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "构建示例插件失败: %v\n%s", err, output)
	return pluginsDir
}

// pluginModuleRoot 定位 backend 模块根（go.mod 所在目录）。
func pluginModuleRoot(t *testing.T) string {
	t.Helper()
	start, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(start, "go.mod")); err == nil {
			return start
		}
		parent := filepath.Dir(start)
		if parent == start {
			break
		}
		start = parent
	}
	t.Fatal("未找到 backend 模块根（go.mod）")
	return ""
}

// waitStatus 轮询插件运行态直到满足条件或超时。
func waitStatus(t *testing.T, m *Manager, id, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if m.Status(id) == want {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("插件 %s 状态 %s 未在 %s 内出现（当前 %s）", id, want, timeout, m.Status(id))
}

func TestManager_Integration_Lifecycle(t *testing.T) {
	pluginsDir := t.TempDir()
	buildHelloPlugin(t, pluginsDir)

	repo := memory.NewPluginRepository()
	m := NewManager(pluginsDir, repo, "test")
	require.NoError(t, m.Load(t.Context()))

	// 新插件默认禁用
	p, err := repo.FindByID("hello")
	require.NoError(t, err)
	assert.False(t, p.IsActive)

	// 启用 → spawn → running + HTTP 反代可用
	require.NoError(t, m.Enable(t.Context(), "hello"))
	waitStatus(t, m, "hello", StatusRunning, 30*time.Second)

	addr := m.HTTPAddress("hello")
	require.NotEmpty(t, addr, "运行中插件应暴露回环 HTTP 地址")
	resp, err := http.Get("http://" + addr + "/healthz")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// API 数据面
	resp, err = http.Get("http://" + addr + "/api/hello")
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// 停用 → 停止进程
	require.NoError(t, m.Disable(t.Context(), "hello"))
	waitStatus(t, m, "hello", StatusStopped, 15*time.Second)
	assert.Equal(t, "", m.HTTPAddress("hello"))
	assert.False(t, func() bool {
		p, _ := repo.FindByID("hello")
		return p.IsActive
	}(), "停用后 is_active=false")

	// 再启用 → 重启链路
	require.NoError(t, m.Enable(t.Context(), "hello"))
	waitStatus(t, m, "hello", StatusRunning, 30*time.Second)
	require.NoError(t, m.Restart(t.Context(), "hello"))
	waitStatus(t, m, "hello", StatusRunning, 30*time.Second)

	// 优雅停机
	m.Shutdown(t.Context())
	waitStatus(t, m, "hello", StatusStopped, 15*time.Second)
}
