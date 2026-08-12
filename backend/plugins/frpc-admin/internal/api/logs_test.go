package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLogsJournalFallback 默认配置无真实日志路径（INI 无 log_file）→ 走 journalctl，
// unit 使用默认 frpc。
func TestLogsJournalFallback(t *testing.T) {
	_, srv, hs := newTestApp(t)
	srv.setFile("/etc/frp/frpc.ini", "[common]\nserver_addr = 1.2.3.4\nserver_port = 7000\n")

	resp := doReq(t, "GET", hs.URL+"/api/logs", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out LogsResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.Equal(t, "journal", out.Source)
	assert.Equal(t, "frpc", out.Path)
	assert.Equal(t, defaultLogLines, out.Lines)
	assert.NotEmpty(t, out.Content)
	assert.Contains(t, out.Content, "login to server success")
	assert.Empty(t, out.Error)
}

// TestLogsJournalUnitFromRestartCmd 重启命令指定 unit（systemctl restart portal-frpc）
// → journal 使用该 unit。
func TestLogsJournalUnitFromRestartCmd(t *testing.T) {
	app, srv, hs := newTestApp(t)
	conn, _ := app.store.Get()
	conn.RestartCmd = "systemctl restart portal-frpc"
	require.NoError(t, app.store.Save(conn))
	srv.setFile("/etc/frp/frpc.ini", "[common]\nserver_addr = 1.2.3.4\n")

	resp := doReq(t, "GET", hs.URL+"/api/logs", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out LogsResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.Equal(t, "journal", out.Source)
	assert.Equal(t, "portal-frpc", out.Path, "应从重启命令提取 unit")
}

// TestLogsFileINI 配置声明 log_file（INI 旧版）→ 走 tail 读取该文件。
func TestLogsFileINI(t *testing.T) {
	_, srv, hs := newTestApp(t)
	srv.setFile("/etc/frp/frpc.ini", "[common]\nserver_addr = 1.2.3.4\nlog_file = /var/log/frpc.log\n")
	srv.setFile("/var/log/frpc.log", "line1\nline2\n")

	resp := doReq(t, "GET", hs.URL+"/api/logs", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out LogsResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.Equal(t, "file", out.Source)
	assert.Equal(t, "/var/log/frpc.log", out.Path)
	assert.Equal(t, "line1\nline2", out.Content)
}

// TestLogsFileTOML 配置声明 log.to（TOML 新版）→ 走 tail 读取该文件。
func TestLogsFileTOML(t *testing.T) {
	app, srv, hs := newTestApp(t)
	conn, _ := app.store.Get()
	conn.ConfigPath = "/etc/frp/frpc.toml"
	conn.Format = "toml"
	require.NoError(t, app.store.Save(conn))
	srv.setFile("/etc/frp/frpc.toml", "serverAddr = \"1.2.3.4\"\nserverPort = 7000\n\n[log]\nto = \"/var/log/frpc.log\"\nlevel = \"info\"\n")
	srv.setFile("/var/log/frpc.log", "t1\nt2\n")

	resp := doReq(t, "GET", hs.URL+"/api/logs", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out LogsResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.Equal(t, "file", out.Source)
	assert.Equal(t, "/var/log/frpc.log", out.Path)
	assert.Equal(t, "t1\nt2", out.Content)
}

// TestLogsConsoleIsJournal TOML log.to = console（不落盘）→ 视为无日志文件，走 journal。
func TestLogsConsoleIsJournal(t *testing.T) {
	app, srv, hs := newTestApp(t)
	conn, _ := app.store.Get()
	conn.ConfigPath = "/etc/frp/frpc.toml"
	conn.Format = "toml"
	require.NoError(t, app.store.Save(conn))
	srv.setFile("/etc/frp/frpc.toml", "serverAddr = \"1.2.3.4\"\n\n[log]\nto = \"console\"\n")
	srv.setFile("/var/log/frpc.log", "不应被读到")

	resp := doReq(t, "GET", hs.URL+"/api/logs", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out LogsResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.Equal(t, "journal", out.Source, "console 输出视为 journal 日志")
}

// TestLogsLinesClamp lines 参数钳制在 [50, 2000]。
func TestLogsLinesClamp(t *testing.T) {
	_, _, hs := newTestApp(t)
	for _, tc := range []struct {
		q      string
		expect int
	}{
		{"10", minLogLines},
		{"50", 50},
		{"200", 200},
		{"5000", maxLogLines},
		{"abc", defaultLogLines},
		{"", defaultLogLines},
	} {
		url := hs.URL + "/api/logs"
		if tc.q != "" {
			url += "?lines=" + tc.q
		}
		resp := doReq(t, "GET", url, "")
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var out LogsResponse
		require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
		assert.Equal(t, tc.expect, out.Lines, "lines=%q 应钳制到 %d", tc.q, tc.expect)
	}
}

// TestLogsFileMissing 声明的日志文件不存在 → 返回错误信息但 200（前端可展示）。
func TestLogsFileMissing(t *testing.T) {
	_, srv, hs := newTestApp(t)
	srv.setFile("/etc/frp/frpc.ini", "[common]\nserver_addr = 1.2.3.4\nlog_file = /var/log/frpc.log\n")

	resp := doReq(t, "GET", hs.URL+"/api/logs", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out LogsResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.Equal(t, "file", out.Source)
	assert.NotEmpty(t, out.Error, "文件不存在应报错")
}

// TestLogsNoConnection 未配置连接 → 404。
func TestLogsNoConnection(t *testing.T) {
	dir := t.TempDir()
	app := NewApp(mustStore(t, dir), "")
	mux := newMux(app)
	resp := doReq(t, "GET", mux.URL+"/api/logs", "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// TestLogCommandFile / TestLogCommandJournal 校验日志命令构造（含单引号转义）。
func TestLogCommandFile(t *testing.T) {
	c := logCommandFor(logSource{Mode: "file", Path: "/var/log/frpc.log"}, 200)
	assert.Equal(t, "tail -n 200 '/var/log/frpc.log'", c)
	c = logCommandFor(logSource{Mode: "file", Path: "/a' b.log"}, 50)
	assert.Equal(t, "tail -n 50 '/a'\\'' b.log'", c)
}

func TestLogCommandJournal(t *testing.T) {
	c := logCommandFor(logSource{Mode: "journal", Path: "frpc"}, 200)
	assert.Equal(t, "journalctl -u 'frpc' -n 200 --no-pager", c)
}

// TestSystemdUnitFromRestartCmd 校验 unit 提取：无配置/无匹配回退 frpc。
func TestSystemdUnitFromRestartCmd(t *testing.T) {
	assert.Equal(t, "frpc", systemdUnitFrom(""))
	assert.Equal(t, "frpc", systemdUnitFrom("docker restart f"))
	assert.Equal(t, "my-frpc", systemdUnitFrom("sudo systemctl restart my-frpc"))
	assert.Equal(t, "frpc", systemdUnitFrom("echo hi"))
}

// TestParseBackupStat 校验 stat 输出解析（只保留 .bak.<ts>，忽略非备份名）。
// 排序发生在 handler（handleListBackups），解析函数保持输入顺序。
func TestParseBackupStat(t *testing.T) {
	out := "/etc/frp/frpc.ini.bak.1700000000 123\n/etc/frp/frpc.ini.bak.1700000001 45\n/etc/frp/frpc.ini.copy 99\nbad line\n"
	list := parseBackupStat(out)
	require.Len(t, list, 2)
	assert.Equal(t, "1700000000", list[0].TS)
	assert.Equal(t, int64(123), list[0].Size)
	assert.Equal(t, "1700000001", list[1].TS)
	assert.Equal(t, int64(45), list[1].Size)
}

// TestBackupTSFromPath 校验备份路径后缀提取。
func TestBackupTSFromPath(t *testing.T) {
	ts, ok := backupTSFromPath("/a/frpc.ini.bak.1700000000")
	assert.True(t, ok)
	assert.Equal(t, "1700000000", ts)
	_, ok = backupTSFromPath("/a/frpc.ini.bak.backup")
	assert.False(t, ok)
	_, ok = backupTSFromPath("/a/frpc.ini")
	assert.False(t, ok)
}

// TestLogsStringPathNeverEmitted 校验日志命令构造（含单引号转义）。
func TestLogsStringPathNeverEmitted(t *testing.T) {
	c := logCommandFor(logSource{Mode: "file", Path: "/p a th"}, 100)
	assert.NotContains(t, strings.Fields(c), "/p a th", "路径应被单引号包裹，不能裸奔")
}

// TestLogsFormatMismatchDetectFallback 回归：连接格式提示与实际不符（提示 ini 实为
// TOML）时，应按内容自动检测而非误降 journal（S-3 建议）。
func TestLogsFormatMismatchDetectFallback(t *testing.T) {
	app, srv, hs := newTestApp(t)
	conn, _ := app.store.Get()
	conn.Format = "ini" // 错误提示：实际是 TOML
	conn.ConfigPath = "/etc/frp/frpc.toml"
	require.NoError(t, app.store.Save(conn))
	srv.setFile("/etc/frp/frpc.toml", "serverAddr = \"1.2.3.4\"\nserverPort = 7000\n\n[log]\nto = \"/var/log/frpc.log\"\n")
	srv.setFile("/var/log/frpc.log", "detect-fallback-line\n")

	resp := doReq(t, "GET", hs.URL+"/api/logs", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out LogsResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.Equal(t, "file", out.Source, "格式提示不符时也应自动检测出日志文件")
	assert.Equal(t, "/var/log/frpc.log", out.Path)
	assert.Contains(t, out.Content, "detect-fallback-line")
}

// TestLogsTOMLFileSchemePrefix 回归：TOML log.to 采用 file:// 前缀时剥去后仍可 tail。
func TestLogsTOMLFileSchemePrefix(t *testing.T) {
	app, srv, hs := newTestApp(t)
	conn, _ := app.store.Get()
	conn.ConfigPath = "/etc/frp/frpc.toml"
	conn.Format = "toml"
	require.NoError(t, app.store.Save(conn))
	srv.setFile("/etc/frp/frpc.toml", "serverAddr = \"1.2.3.4\"\n\n[log]\nto = \"file:///var/log/frpc.log\"\n")
	srv.setFile("/var/log/frpc.log", "scheme-line\n")

	resp := doReq(t, "GET", hs.URL+"/api/logs", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out LogsResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.Equal(t, "file", out.Source)
	assert.Equal(t, "/var/log/frpc.log", out.Path, "file:// 前缀应被剥离")
	assert.Contains(t, out.Content, "scheme-line")
}