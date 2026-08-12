package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testConfigPath = "/etc/frp/frpc.ini"
	testBakOld     = "/etc/frp/frpc.ini.bak.1700000000"
	testBakNew     = "/etc/frp/frpc.ini.bak.1700000001"
)

func TestListBackups(t *testing.T) {
	_, srv, hs := newTestApp(t)
	srv.setFile(testBakOld, "old-backup-content")
	srv.setFile(testBakNew, "new-content-here")

	resp := doReq(t, "GET", hs.URL+"/api/backups", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out ListBackupsResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	require.Len(t, out.Backups, 2)
	// 按时间戳倒序
	assert.Equal(t, "1700000001", out.Backups[0].TS)
	assert.Equal(t, int64(len("new-content-here")), out.Backups[0].Size)
	assert.Equal(t, "1700000000", out.Backups[1].TS)
	assert.Equal(t, testBakNew, out.Backups[0].Path)
}

func TestListBackupsEmpty(t *testing.T) {
	_, _, hs := newTestApp(t)
	resp := doReq(t, "GET", hs.URL+"/api/backups", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out ListBackupsResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.Empty(t, out.Backups)
}

func TestListBackupsNoConnection(t *testing.T) {
	dir := t.TempDir()
	app := NewApp(mustStore(t, dir), "")
	mux := newMux(app)
	resp := doReq(t, "GET", mux.URL+"/api/backups", "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetBackup(t *testing.T) {
	_, srv, hs := newTestApp(t)
	srv.setFile(testBakOld, "[common]\nserver_addr = 7.7.7.7\n")

	resp := doReq(t, "GET", hs.URL+"/api/backups/1700000000", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out BackupContentResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.Equal(t, testBakOld, out.Path)
	assert.Equal(t, "1700000000", out.TS)
	assert.Equal(t, int64(len("[common]\nserver_addr = 7.7.7.7\n")), out.Size)
	assert.Contains(t, out.Content, "7.7.7.7")
}

func TestGetBackupInvalidTS(t *testing.T) {
	_, _, hs := newTestApp(t)
	for _, ts := range []string{"abc", "1700000000x", "1.5"} {
		resp := doReq(t, "GET", hs.URL+"/api/backups/"+ts, "")
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "ts=%q 应 400", ts)
	}
	// 路径穿越片段被路由层清理后不匹配，应返回非 200（Go ServeMux 清洗 ../）
	resp := doReq(t, "GET", hs.URL+"/api/backups/../etc", "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode, "穿越路径应被拒绝")
}

func TestGetBackupNotFound(t *testing.T) {
	_, _, hs := newTestApp(t)
	resp := doReq(t, "GET", hs.URL+"/api/backups/9999999999", "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestRestoreBackupSuccess(t *testing.T) {
	_, srv, hs := newTestApp(t)
	srv.setFile(testConfigPath, "[common]\nserver_addr = 1.2.3.4\n")
	srv.setFile(testBakOld, "[common]\nserver_addr = 8.8.8.8\n")

	resp := doReq(t, "POST", hs.URL+"/api/backups/1700000000/restore", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out SaveConfigResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.True(t, out.Applied)
	assert.False(t, out.RolledBack)
	assert.Empty(t, out.Error)
	assert.NotEmpty(t, out.BackupPath, "恢复前应备份当前配置")
	assert.Contains(t, out.BackupPath, ".bak.")
	assert.Contains(t, out.RestartOutput, "restarted")

	content, ok := srv.getFile(testConfigPath)
	require.True(t, ok)
	assert.Contains(t, content, "8.8.8.8", "配置应被恢复为所选备份")
	// 恢复前的当前配置应已备份（可回退）
	cur, ok := srv.getFile(out.BackupPath)
	require.True(t, ok)
	assert.Contains(t, cur, "1.2.3.4")
}

func TestRestoreBackupRestartFailRollback(t *testing.T) {
	_, srv, hs := newTestApp(t)
	srv.setFile(testConfigPath, "[common]\nserver_addr = 1.2.3.4\n")
	srv.setFile(testBakOld, "[common]\nserver_addr = 8.8.8.8\n")
	srv.failRestart = true

	resp := doReq(t, "POST", hs.URL+"/api/backups/1700000000/restore", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out SaveConfigResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.False(t, out.Applied)
	assert.True(t, out.RolledBack, "重启失败应回滚")
	assert.NotEmpty(t, out.RestartOutput)
	assert.NotEmpty(t, out.BackupPath)

	content, ok := srv.getFile(testConfigPath)
	require.True(t, ok)
	assert.Contains(t, content, "1.2.3.4", "重启失败后应恢复回恢复前的配置")
	assert.NotContains(t, content, "8.8.8.8")
}

func TestRestoreBackupMissing(t *testing.T) {
	_, srv, hs := newTestApp(t)
	srv.setFile(testConfigPath, "[common]\nserver_addr = 1.2.3.4\n")

	resp := doReq(t, "POST", hs.URL+"/api/backups/9999999999/restore", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out SaveConfigResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.Contains(t, out.Error, "不存在")
	assert.False(t, out.Applied)

	// 不存在的备份不应改写原配置
	content, ok := srv.getFile(testConfigPath)
	require.True(t, ok)
	assert.Contains(t, content, "1.2.3.4")
}

func TestRestoreBackupInvalidTS(t *testing.T) {
	_, _, hs := newTestApp(t)
	resp := doReq(t, "POST", hs.URL+"/api/backups/abc/restore", "")
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	// 穿越片段被路由层清洗后不匹配
	resp = doReq(t, "POST", hs.URL+"/api/backups/../../etc/restore", "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestRestoreBackupWriteFailRollback(t *testing.T) {
	_, srv, hs := newTestApp(t)
	srv.setFile(testConfigPath, "[common]\nserver_addr = 1.2.3.4\n")
	srv.setFile(testBakOld, "[common]\nserver_addr = 8.8.8.8\n")
	srv.failRestoreCP = true

	resp := doReq(t, "POST", hs.URL+"/api/backups/1700000000/restore", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out SaveConfigResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.Contains(t, out.Error, "恢复备份失败")
	assert.True(t, out.RolledBack, "恢复写入失败应回滚到恢复前配置")
	assert.NotEmpty(t, out.BackupPath)

	content, ok := srv.getFile(testConfigPath)
	require.True(t, ok)
	assert.Contains(t, content, "1.2.3.4", "恢复失败后应恢复当前配置")
	assert.NotContains(t, content, "8.8.8.8")
}

func TestRestoreBackupNoConnection(t *testing.T) {
	dir := t.TempDir()
	app := NewApp(mustStore(t, dir), "")
	mux := newMux(app)
	resp := doReq(t, "POST", mux.URL+"/api/backups/1700000000/restore", "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestBackupPathForFixed(t *testing.T) {
	p := backupPathFor("/etc/frp/frpc.ini", "1700000000")
	assert.Equal(t, "/etc/frp/frpc.ini.bak.1700000000", p)
}

// TestBackupSudoWrapping 少量回归：启用 sudo 时 list/read 命令应经 sudo 包裹
// 并正常返回（测试服务器经 sudo -S 分支递归执行）。
func TestBackupSudoWrapping(t *testing.T) {
	app, srv, hs := newTestApp(t)
	conn, _ := app.store.Get()
	conn.SudoEnabled = true
	conn.SudoPassword = "sudopw"
	require.NoError(t, app.store.Save(conn))
	srv.setFile(testBakOld, "sudo-backup")
	srv.setFile(testConfigPath, "[common]\n")

	resp := doReq(t, "GET", hs.URL+"/api/backups", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out ListBackupsResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	require.Len(t, out.Backups, 1)
	assert.Equal(t, "1700000000", out.Backups[0].TS)

	resp = doReq(t, "GET", hs.URL+"/api/backups/1700000000", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var got BackupContentResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &got))
	assert.Contains(t, got.Content, "sudo-backup")
}