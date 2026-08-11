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

func TestGetConnectionEmpty(t *testing.T) {
	dir := t.TempDir()
	store := mustStore(t, dir)
	app := NewApp(store, "")
	mux := newMux(app)
	resp := doReq(t, "GET", mux.URL+"/api/connection", "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestSaveAndGetConnection(t *testing.T) {
	_, _, hs := newTestApp(t)
	body := `{"host":"10.0.0.10","port":22,"user":"root","password":"pw","sudo_enabled":true,"config_path":"/etc/frp/frpc.ini","format":"ini"}`
	resp := doReq(t, "PUT", hs.URL+"/api/connection", body)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	out := readBody(t, resp)
	assert.NotContains(t, out, "pw", "密码应脱敏")

	resp = doReq(t, "GET", hs.URL+"/api/connection", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var conn struct {
		Host       string `json:"host"`
		Password   string `json:"password"`
		ConfigPath string `json:"config_path"`
	}
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &conn))
	assert.Equal(t, "10.0.0.10", conn.Host)
	assert.Equal(t, "", conn.Password, "GET 不应回显密码")
	assert.Equal(t, "/etc/frp/frpc.ini", conn.ConfigPath)
}

func TestSaveConnectionKeepsPasswordOnEmpty(t *testing.T) {
	app, _, hs := newTestApp(t)
	body := `{"host":"10.0.0.10","port":22,"user":"root","password":"secret","sudo_password":"sudosec"}`
	resp := doReq(t, "PUT", hs.URL+"/api/connection", body)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// 第二次保存不携带密码（前端不回显），应沿用旧值
	body = `{"host":"10.0.0.10","port":2222,"user":"root"}`
	resp = doReq(t, "PUT", hs.URL+"/api/connection", body)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// 直接读存储层确认密码仍在（API 层脱敏不可见）
	got, ok := app.store.Get()
	require.True(t, ok)
	assert.Equal(t, "secret", got.Password, "空密码保存应沿用旧值")
	assert.Equal(t, "sudosec", got.SudoPassword, "空 sudo 密码保存应沿用旧值")
	assert.Equal(t, 2222, got.Port, "非凭据字段应更新")
}

func TestDeleteConnection(t *testing.T) {
	_, _, hs := newTestApp(t)
	resp := doReq(t, "DELETE", hs.URL+"/api/connection", "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp = doReq(t, "DELETE", hs.URL+"/api/connection", "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetConfig(t *testing.T) {
	_, srv, hs := newTestApp(t)
	srv.setFile("/etc/frp/frpc.ini", "[common]\nserver_addr = 1.2.3.4\nserver_port = 7000\n\n[ssh]\ntype = tcp\nlocal_port = 22\nremote_port = 6000\n")

	resp := doReq(t, "GET", hs.URL+"/api/config", "")
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

// TestGetConfigTOMLAutoFormat 回归：连接格式为 auto 时读取 TOML 配置必须自动检测
// 为 toml 并解析全部 [[proxies]]（旧实现把 auto 误回退为 ini，导致 TOML 被 INI
// 解析器错误解析成单个 [proxies] 代理，见 config.go resolveFormat）。
func TestGetConfigTOMLAutoFormat(t *testing.T) {
	app, srv, hs := newTestApp(t)
	conn, _ := app.store.Get()
	conn.Format = "auto"
	require.NoError(t, app.store.Save(conn))
	srv.setFile("/etc/frp/frpc.ini", `serverAddr = "114.55.138.23"
serverPort = 7000
auth.token = "secret"

[[proxies]]
name = "rdp"
type = "tcp"
localIP = "127.0.0.1"
localPort = 3389
remotePort = 13389

[[proxies]]
name = "minecraft"
type = "tcp"
localIP = "127.0.0.1"
localPort = 25565
remotePort = 25565

[[proxies]]
name = "ssh"
type = "tcp"
localIP = "127.0.0.1"
localPort = 22
remotePort = 12222
`)

	resp := doReq(t, "GET", hs.URL+"/api/config", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out ConfigResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.Equal(t, "toml", out.Format, "auto 应自动检测为 toml")
	server, ok := out.Server.(map[string]any)
	require.True(t, ok, "server 应为对象")
	assert.Equal(t, "114.55.138.23", server["server_addr"])
	assert.EqualValues(t, 7000, server["server_port"])
	require.Len(t, out.Proxies, 3, "应解析出全部 3 个 [[proxies]]")
	assert.Equal(t, "rdp", out.Proxies[0].Name)
	assert.Equal(t, 13389, out.Proxies[0].RemotePort)
	assert.Equal(t, "ssh", out.Proxies[2].Name)
	assert.Equal(t, 12222, out.Proxies[2].RemotePort)
	assert.NotEqual(t, "[proxies]", out.Proxies[0].Name, "不应出现 [proxies] 字面量作为代理名")
}

func TestGetConfigNoConnection(t *testing.T) {
	dir := t.TempDir()
	store := mustStore(t, dir)
	app := NewApp(store, "")
	mux := newMux(app)
	resp := doReq(t, "GET", mux.URL+"/api/config", "")
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestGetConfigParseError(t *testing.T) {
	_, srv, hs := newTestApp(t)
	srv.setFile("/etc/frp/frpc.ini", "[common\nserver_addr = 1.2.3.4")
	resp := doReq(t, "GET", hs.URL+"/api/config", "")
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
	assert.Contains(t, readBody(t, resp), "content")
}

func TestProbe(t *testing.T) {
	_, _, hs := newTestApp(t)
	resp := doReq(t, "POST", hs.URL+"/api/probe", "")
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
	resp := doReq(t, "PUT", hs.URL+"/api/config", reqBody)
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
	resp := doReq(t, "PUT", hs.URL+"/api/config", reqBody)
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
	resp := doReq(t, "PUT", hs.URL+"/api/config", `{"content":"serverAddr = [broken","format":"toml"}`)
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
	resp := doReq(t, "PUT", hs.URL+"/api/config", reqBody)
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

	resp := doReq(t, "PUT", hs.URL+"/api/config", `{"content":"[common]\nserver_addr = 9.9.9.9\n","format":"ini"}`)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out SaveConfigResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.False(t, out.Applied)
}

func TestPutConfigNoContent(t *testing.T) {
	_, _, hs := newTestApp(t)
	resp := doReq(t, "PUT", hs.URL+"/api/config", `{}`)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestGetConfigAutoFormatINI 回归：连接格式 auto + INI 内容必须仍按 INI 解析，
// 防止 resolveFormat 改回 auto 后误伤旧 INI 配置。
func TestGetConfigAutoFormatINI(t *testing.T) {
	app, srv, hs := newTestApp(t)
	conn, _ := app.store.Get()
	conn.Format = "auto"
	require.NoError(t, app.store.Save(conn))
	srv.setFile("/etc/frp/frpc.ini", "[common]\nserver_addr = 1.2.3.4\nserver_port = 7000\n\n[ssh]\ntype = tcp\nlocal_port = 22\nremote_port = 6000\n")

	resp := doReq(t, "GET", hs.URL+"/api/config", "")
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out ConfigResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.Equal(t, "ini", out.Format, "auto 应检测出 INI 格式")
	require.Len(t, out.Proxies, 1)
	assert.Equal(t, "ssh", out.Proxies[0].Name)
	assert.Equal(t, 6000, out.Proxies[0].RemotePort)
}

// TestPutConfigRawAutoTOML 回归：raw 模式 + format=auto 保存 TOML 原文，
// SyntaxCheck 内部 Detect 应识别为 toml（前端 raw 保存恒发 auto）。
func TestPutConfigRawAutoTOML(t *testing.T) {
	_, srv, hs := newTestApp(t)
	srv.setFile("/etc/frp/frpc.ini", "serverAddr = \"1.2.3.4\"\nserverPort = 7000\n")

	reqBody := `{"content":"serverAddr = \"9.9.9.9\"\nserverPort = 7000\n\nauth.token = \"tok\"\n\n[[proxies]]\nname = \"ssh\"\ntype = \"tcp\"\nlocalIP = \"127.0.0.1\"\nlocalPort = 22\nremotePort = 6000\n","format":"auto"}`
	resp := doReq(t, "PUT", hs.URL+"/api/config", reqBody)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out SaveConfigResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.True(t, out.SyntaxOK, "TOML 原文 + auto 应通过语法检查: %s", out.SyntaxError)
	assert.True(t, out.Applied)
	assert.False(t, out.RolledBack)

	content, ok := srv.getFile("/etc/frp/frpc.ini")
	require.True(t, ok)
	assert.Contains(t, content, "serverAddr")
	assert.Contains(t, content, "9.9.9.9")
}

// TestPutConfigStructuredHTTPDropsRemotePort 回归：结构化保存 http/https 代理时，
// 即使携带 remote_port 也不渲染该键（frp http/https 类型不支持 remote port，命中
// 会导致 frpc 启动失败；可视化界面也会按类型隐藏该字段，此处为后端兜底）。
func TestPutConfigStructuredHTTPDropsRemotePort(t *testing.T) {
	app, srv, hs := newTestApp(t)
	srv.setFile("/etc/frp/frpc.ini", "[common]\nserver_addr = 1.2.3.4\nserver_port = 7000\n")

	// 结构化保存：http 代理带了 remote_port（模拟可视化界面上残留/误填）
	reqBody := `{"structured":{"format":"ini","server":{"server_addr":"1.2.3.4","server_port":7000},"proxies":[{"name":"web","type":"http","local_ip":"127.0.0.1","local_port":8080,"remote_port":8800,"custom_domains":["app.example.com"]}]}}`
	resp := doReq(t, "PUT", hs.URL+"/api/config", reqBody)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out SaveConfigResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.True(t, out.SyntaxOK, "结构化保存应通过语法检查: %s", out.SyntaxError)
	assert.True(t, out.Applied)

	content, ok := srv.getFile("/etc/frp/frpc.ini")
	require.True(t, ok)
	assert.Contains(t, content, "custom_domains = app.example.com")
	assert.NotContains(t, content, "remote_port", "http 代理落盘不应含 remote_port")
	assert.NotContains(t, content, "8800", "http 代理的 remote_port 值不应落盘")

	// TOML 版本（切换连接指向 .toml 配置）
	conn, _ := app.store.Get()
	conn.ConfigPath = "/etc/frp/frpc.toml"
	conn.Format = "toml"
	require.NoError(t, app.store.Save(conn))
	srv.setFile("/etc/frp/frpc.toml", "serverAddr = \"1.2.3.4\"\nserverPort = 7000\n")
	reqBody = `{"structured":{"format":"toml","server":{"server_addr":"1.2.3.4","server_port":7000},"proxies":[{"name":"web","type":"http","local_ip":"127.0.0.1","local_port":8080,"remote_port":8800,"custom_domains":["app.example.com"]}]}}`
	resp = doReq(t, "PUT", hs.URL+"/api/config", reqBody)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	out2 := SaveConfigResponse{}
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out2))
	assert.True(t, out2.Applied)

	tcontent, ok := srv.getFile("/etc/frp/frpc.toml")
	require.True(t, ok)
	assert.Contains(t, tcontent, "customDomains")
	assert.NotContains(t, tcontent, "remotePort", "http 代理 TOML 落盘不应含 remotePort")
	assert.NotContains(t, tcontent, "8800")
}

// TestPutConfigStructuredAutoFormat 回归：结构化保存未显式指定格式 + 连接格式
// auto 时，应回退为 ini（Detect 空原文默认 ini）而不触发 Render 的 auto 报错。
func TestPutConfigStructuredAutoFormat(t *testing.T) {
	app, srv, hs := newTestApp(t)
	conn, _ := app.store.Get()
	conn.Format = "auto"
	require.NoError(t, app.store.Save(conn))
	srv.setFile("/etc/frp/frpc.ini", "[common]\nserver_addr = 1.2.3.4\n")

	reqBody := `{"structured":{"server":{"server_addr":"1.2.3.4","server_port":7000,"token":"tok"},"proxies":[{"name":"ssh","type":"tcp","local_ip":"127.0.0.1","local_port":22,"remote_port":6000}]}}`
	resp := doReq(t, "PUT", hs.URL+"/api/config", reqBody)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var out SaveConfigResponse
	require.NoError(t, json.Unmarshal([]byte(readBody(t, resp)), &out))
	assert.True(t, out.SyntaxOK, "结构化 + 未指定格式应能保存: %s", out.SyntaxError)
	assert.True(t, out.Applied)

	content, ok := srv.getFile("/etc/frp/frpc.ini")
	require.True(t, ok)
	assert.Contains(t, content, "server_addr = 1.2.3.4")
}
