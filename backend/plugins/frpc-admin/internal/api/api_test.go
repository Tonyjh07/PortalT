package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthz(t *testing.T) {
	_, _, hs := newTestApp(t)
	resp := doReq(t, "GET", hs.URL+"/healthz", "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, readBody(t, resp), "ok")
}

// TestStaticNoRedirect 验证静态前端 / 与 /index.html 直接返回 200，
// 不产生 301（Go 1.26 FileServer 会对 /index.html 重定向 ./，形成循环）。
func TestStaticNoRedirect(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>hi</html>"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "assets"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("console.log(1)"), 0o644))

	store := mustStore(t, t.TempDir())
	app := NewApp(store, dir)
	mux := newMux(app)

	for _, p := range []string{"/", "/index.html", "/assets/app.js"} {
		resp := doReq(t, "GET", mux.URL+p, "")
		assert.Equal(t, http.StatusOK, resp.StatusCode, "路径 %s 应返回 200", p)
	}

	// 目录/不存在/穿越均应 404
	for _, p := range []string{"/nope", "/assets", "/../secret"} {
		resp := doReq(t, "GET", mux.URL+p, "")
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, "路径 %s 应返回 404", p)
	}
}

func TestListConnectionsEmpty(t *testing.T) {
	dir := t.TempDir()
	store := mustStore(t, dir)
	app := NewApp(store, "")
	mux := newMux(app)
	resp := doReq(t, "GET", mux.URL+"/api/connections", "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Contains(t, readBody(t, resp), "[]")
}

func TestSaveAndListConnections(t *testing.T) {
	_, _, hs := newTestApp(t)
	body := `{"host":"10.0.0.10","port":22,"user":"root","password":"pw","sudo_enabled":true,"config_path":"/etc/frp/frpc.ini","format":"ini"}`
	resp := doReq(t, "PUT", hs.URL+"/api/connections/vm-2", body)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	out := readBody(t, resp)
	assert.NotContains(t, out, "pw", "密码应脱敏")

	resp = doReq(t, "GET", hs.URL+"/api/connections", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var list []map[string]any
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &list))
	require.Len(t, list, 2)
}

func TestSaveConnectionKeepsPasswordOnEmpty(t *testing.T) {
	app, _, hs := newTestApp(t)
	body := `{"host":"10.0.0.10","port":22,"user":"root","password":"secret","sudo_password":"sudosec"}`
	resp := doReq(t, "PUT", hs.URL+"/api/connections/vm-keep", body)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// 第二次保存不携带密码（前端不回显），应沿用旧值
	body = `{"host":"10.0.0.10","port":2222,"user":"root"}`
	resp = doReq(t, "PUT", hs.URL+"/api/connections/vm-keep", body)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// 直接读存储层确认密码仍在（API 层脱敏不可见）
	got, ok := app.store.Get("vm-keep")
	require.True(t, ok)
	assert.Equal(t, "secret", got.Password, "空密码保存应沿用旧值")
	assert.Equal(t, "sudosec", got.SudoPassword, "空 sudo 密码保存应沿用旧值")
	assert.Equal(t, 2222, got.Port, "非凭据字段应更新")
}

func TestDeleteConnection(t *testing.T) {
	_, _, hs := newTestApp(t)
	resp := doReq(t, "DELETE", hs.URL+"/api/connections/vm-1", "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp = doReq(t, "DELETE", hs.URL+"/api/connections/vm-1", "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetConfig(t *testing.T) {
	_, srv, hs := newTestApp(t)
	srv.setFile("/etc/frp/frpc.ini", "[common]\nserver_addr = 1.2.3.4\nserver_port = 7000\n\n[ssh]\ntype = tcp\nlocal_port = 22\nremote_port = 6000\n")

	resp := doReq(t, "GET", hs.URL+"/api/vms/vm-1/config", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out ConfigResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.Equal(t, "ini", out.Format)
	assert.Equal(t, "/etc/frp/frpc.ini", out.Path)
	assert.Contains(t, out.Content, "server_addr = 1.2.3.4")
	require.Len(t, out.Proxies, 1)
	assert.Equal(t, "ssh", out.Proxies[0].Name)
	assert.Equal(t, 6000, out.Proxies[0].RemotePort)
}

func TestGetConfigNoConnection(t *testing.T) {
	_, _, hs := newTestApp(t)
	resp := doReq(t, "GET", hs.URL+"/api/vms/vm-nope/config", "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetConfigParseError(t *testing.T) {
	_, srv, hs := newTestApp(t)
	srv.setFile("/etc/frp/frpc.ini", "[common\nserver_addr = 1.2.3.4")
	resp := doReq(t, "GET", hs.URL+"/api/vms/vm-1/config", "")
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	assert.Contains(t, readBody(t, resp), "content")
}

func TestProbe(t *testing.T) {
	_, _, hs := newTestApp(t)
	resp := doReq(t, "GET", hs.URL+"/api/vms/vm-1/probe", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var p struct {
		Version    string `json:"version"`
		ConfigPath string `json:"config_path"`
		FormatHint string `json:"format_hint"`
	}
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &p))
	assert.Equal(t, "0.54.0", p.Version)
	assert.Equal(t, "/etc/frp/frpc.ini", p.ConfigPath)
}

func TestPutConfigRawSuccess(t *testing.T) {
	_, srv, hs := newTestApp(t)
	srv.setFile("/etc/frp/frpc.ini", "[common]\nserver_addr = 1.2.3.4\nserver_port = 7000\n")

	reqBody := `{"content":"[common]\nserver_addr = 9.9.9.9\nserver_port = 7000\n\n[ssh]\ntype = tcp\nlocal_port = 22\nremote_port = 6000\n","format":"ini"}`
	resp := doReq(t, "PUT", hs.URL+"/api/vms/vm-1/config", reqBody)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out SaveConfigResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.True(t, out.SyntaxOK)
	assert.True(t, out.Applied)
	assert.False(t, out.RolledBack)
	assert.NotEmpty(t, out.BackupPath)
	assert.True(t, strings.Contains(out.BackupPath, ".bak."))

	content, ok := srv.getFile("/etc/frp/frpc.ini")
	require.True(t, ok)
	assert.Contains(t, content, "server_addr = 9.9.9.9")
}

func TestPutConfigStructuredSuccess(t *testing.T) {
	_, srv, hs := newTestApp(t)
	srv.setFile("/etc/frp/frpc.ini", "[common]\nserver_addr = 1.2.3.4\n")

	reqBody := `{"structured":{"format":"ini","server":{"server_addr":"1.2.3.4","server_port":7000,"token":"tok"},"proxies":[{"name":"ssh","type":"tcp","local_ip":"127.0.0.1","local_port":22,"remote_port":6000}]}}`
	resp := doReq(t, "PUT", hs.URL+"/api/vms/vm-1/config", reqBody)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out SaveConfigResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.True(t, out.Applied)

	content, _ := srv.getFile("/etc/frp/frpc.ini")
	assert.Contains(t, content, "server_port = 7000")
	assert.Regexp(t, `token\s*= tok`, content)
	assert.Regexp(t, `remote_port\s*= 6000`, content)
}

func TestPutConfigSyntaxErrorNoWrite(t *testing.T) {
	_, srv, hs := newTestApp(t)
	srv.setFile("/etc/frp/frpc.ini", "[common]\nserver_addr = 1.2.3.4\n")

	before, _ := srv.getFile("/etc/frp/frpc.ini")
	resp := doReq(t, "PUT", hs.URL+"/api/vms/vm-1/config", `{"content":"serverAddr = [broken","format":"toml"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out SaveConfigResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.False(t, out.SyntaxOK)
	assert.NotEmpty(t, out.SyntaxError)
	assert.False(t, out.Applied)

	after, _ := srv.getFile("/etc/frp/frpc.ini")
	assert.Equal(t, before, after, "语法失败不应改盘")
}

func TestPutConfigRestartFailRollback(t *testing.T) {
	_, srv, hs := newTestApp(t)
	srv.setFile("/etc/frp/frpc.ini", "[common]\nserver_addr = 1.2.3.4\nserver_port = 7000\n")
	srv.failRestart = true

	reqBody := `{"content":"[common]\nserver_addr = 9.9.9.9\nserver_port = 7000\n","format":"ini"}`
	resp := doReq(t, "PUT", hs.URL+"/api/vms/vm-1/config", reqBody)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out SaveConfigResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.False(t, out.Applied)
	assert.True(t, out.RolledBack, "重启失败应回滚")
	assert.NotEmpty(t, out.RestartOutput)

	content, _ := srv.getFile("/etc/frp/frpc.ini")
	assert.Contains(t, content, "server_addr = 1.2.3.4", "应恢复备份内容")
}

func TestPutConfigWriteFailRollback(t *testing.T) {
	_, srv, hs := newTestApp(t)
	srv.setFile("/etc/frp/frpc.ini", "[common]\nserver_addr = 1.2.3.4\n")
	srv.failWrite = true

	resp := doReq(t, "PUT", hs.URL+"/api/vms/vm-1/config", `{"content":"[common]\nserver_addr = 9.9.9.9\n","format":"ini"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out SaveConfigResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.False(t, out.Applied)
}

func TestPutConfigNoContent(t *testing.T) {
	_, _, hs := newTestApp(t)
	resp := doReq(t, "PUT", hs.URL+"/api/vms/vm-1/config", `{}`)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}
