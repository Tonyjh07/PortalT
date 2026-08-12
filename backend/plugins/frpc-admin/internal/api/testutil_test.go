package api

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"

	"portalt-plugins/frpc-admin/internal/configstore"
)

// testSSHServer 简化版假 SSH 服务器（按命令返回预设行为）。
// 覆盖本包保存流程所需命令：cat/cp/sudo -S/systemctl restart/rm -f/probe 相关。
type testSSHServer struct {
	mu       sync.Mutex
	files    map[string]string
	restarts []string
	// failRestart 置真时重启命令返回失败（测试回滚）。
	failRestart bool
	// failWrite 置真时 cp 到目标返回失败。
	failWrite bool
	// failRestoreCP 置真时仅恢复流程的 cp（目标为原配置）失败，用于验证回滚。
	failRestoreCP bool
}

func newTestSSHServer() *testSSHServer {
	return &testSSHServer{files: map[string]string{}}
}

func (s *testSSHServer) setFile(path, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[path] = content
}

func (s *testSSHServer) getFile(path string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.files[path]
	return v, ok
}

// start 启动并返回监听地址。
func (s *testSSHServer) start(t *testing.T) string {
	t.Helper()
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(_ ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if string(pass) == "secret" {
				return nil, nil
			}
			return nil, io.ErrUnexpectedEOF
		},
	}
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(priv)
	require.NoError(t, err)
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go s.serve(cfg, ln)
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().String()
}

func (s *testSSHServer) serve(cfg *ssh.ServerConfig, ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			srvConn, chans, reqs, err := ssh.NewServerConn(c, cfg)
			if err != nil {
				_ = c.Close()
				return
			}
			go ssh.DiscardRequests(reqs)
			for newCh := range chans {
				if newCh.ChannelType() != "session" {
					_ = newCh.Reject(ssh.UnknownChannelType, "unsupported")
					continue
				}
				ch, chReqs, err := newCh.Accept()
				if err != nil {
					continue
				}
				go s.handle(srvConn, ch, chReqs)
			}
		}(c)
	}
}

func (s *testSSHServer) handle(_ *ssh.ServerConn, ch ssh.Channel, reqs <-chan *ssh.Request) {
	defer ch.Close()
	for req := range reqs {
		if req.Type != "exec" {
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
			continue
		}
		var p struct{ Command string }
		_ = ssh.Unmarshal(req.Payload, &p)
		_ = req.Reply(true, nil)
		code := s.exec(ch, p.Command)
		_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(code)}))
		return
	}
}

func (s *testSSHServer) exec(ch ssh.Channel, cmd string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.execLocked(ch, cmd)
}

// execLocked 不加锁的命令分发；sudo -S 分支递归调用本函数，避免重复加锁死锁。
func (s *testSSHServer) execLocked(ch ssh.Channel, cmd string) int {
	cmd = strings.TrimSpace(cmd)
	switch {
	case strings.HasPrefix(cmd, "cat > "):
		path := stripQ(strings.TrimPrefix(cmd, "cat > "))
		b, _ := io.ReadAll(ch)
		s.files[path] = string(b)
		return 0
	case strings.HasPrefix(cmd, "chmod "):
		return 0
	case strings.HasPrefix(cmd, "cat "):
		path := stripQ(strings.TrimPrefix(cmd, "cat "))
		if v, ok := s.files[path]; ok {
			_, _ = io.WriteString(ch, v)
			return 0
		}
		_, _ = io.WriteString(ch, "No such file\n")
		return 1
	case strings.HasPrefix(cmd, "sudo -S"):
		rest := strings.TrimPrefix(cmd, "sudo -S")
		if i := strings.Index(rest, "'' "); i >= 0 {
			rest = rest[i+3:]
		}
		// 读密码行（占位）
		_, _ = io.ReadAll(ch)
		return s.execLocked(ch, strings.TrimSpace(rest))
	case strings.HasPrefix(cmd, "cp "):
		parts := strings.Fields(cmd)
		if len(parts) == 3 {
			if s.failWrite {
				_, _ = io.WriteString(ch, "cp: permission denied\n")
				return 1
			}
			dst := stripQ(parts[2])
			if s.failRestoreCP && !strings.Contains(dst, ".bak.") {
				// 仅让"写原配置"这一条 cp 失败一次；后续回滚的 cp 应成功
				s.failRestoreCP = false
				_, _ = io.WriteString(ch, "cp: error writing\n")
				return 1
			}
			if v, ok := s.files[stripQ(parts[1])]; ok {
				s.files[dst] = v
			}
		}
		return 0
	case strings.HasPrefix(cmd, "rm -f "):
		path := stripQ(strings.TrimPrefix(cmd, "rm -f "))
		delete(s.files, path)
		return 0
	case strings.Contains(cmd, "ls -1t"):
		// 备份清理：列出 .bak.* 文件供 tail -n +N 截断（测试无需真实删除）
		prefix := stripQ(partsBetween(cmd, "ls -1t ", ".bak.*"))
		for p := range s.files {
			if strings.HasPrefix(p, prefix+".bak.") {
				_, _ = io.WriteString(ch, p+"\n")
			}
		}
		return 0
	case strings.HasPrefix(cmd, "stat -c "):
		// 备份列表：stat -c '%n %s' '<path>'.bak.* → 每行 "路径 大小"
		prefix := stripQ(partsBetween(cmd, "stat -c '%n %s' ", ".bak.*"))
		for p, v := range s.files {
			if strings.HasPrefix(p, prefix+".bak.") {
				_, _ = io.WriteString(ch, fmt.Sprintf("%s %d\n", p, len(v)))
			}
		}
		return 0
	case strings.HasPrefix(cmd, "tail -n "):
		// 日志文件模式：tail -n <n> '<path>' → 文件内容（读 files 映射）
		path := stripQ(lastField(cmd))
		if v, ok := s.files[path]; ok {
			_, _ = io.WriteString(ch, v)
			return 0
		}
		_, _ = io.WriteString(ch, "No such file\n")
		return 1
	case strings.HasPrefix(cmd, "journalctl "):
		_, _ = io.WriteString(ch, "Jun 03 10:00:01 host frpc[123]: [sshd] 2024/06/03 10:00:01 [I] login to server success\n")
		_, _ = io.WriteString(ch, "Jun 03 10:00:02 host frpc[123]: [sshd] 2024/06/03 10:00:02 [I] start proxy success\n")
		return 0
	case strings.Contains(cmd, "systemctl restart") || strings.Contains(cmd, "restart frpc"):
		if s.failRestart {
			_, _ = io.WriteString(ch, "Job for frpc.service failed\n")
			return 1
		}
		s.restarts = append(s.restarts, cmd)
		_, _ = io.WriteString(ch, "restarted\n")
		return 0
	case strings.Contains(cmd, "frpc --version"):
		_, _ = io.WriteString(ch, "frp 0.54.0\n")
		return 0
	case strings.Contains(cmd, "systemctl cat frpc"):
		_, _ = io.WriteString(ch, "ExecStart=/usr/bin/frpc -c /etc/frp/frpc.ini\n")
		return 0
	case strings.Contains(cmd, "ps -eo"):
		_, _ = io.WriteString(ch, "frpc -c /etc/frp/frpc.ini\n")
		return 0
	case strings.Contains(cmd, "ls -l"):
		_, _ = io.WriteString(ch, "/etc/frp/frpc.ini\n")
		return 0
	default:
		_, _ = io.WriteString(ch, "command not found\n")
		return 127
	}
}

func stripQ(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}
	return s
}

// partsBetween 提取命令中位于 prefix 与 suffix 之间的片段（不含首尾）。
func partsBetween(cmd, prefix, suffix string) string {
	start := strings.Index(cmd, prefix)
	if start < 0 {
		return ""
	}
	s := cmd[start+len(prefix):]
	if i := strings.Index(s, suffix); i >= 0 {
		s = s[:i]
	}
	return s
}

// lastField 返回命令的最后一个空白分隔字段（tail -n 5 '<path>' → '<path>'）。
func lastField(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}

// newTestApp 构造带内存存储 + 假 SSH 服务器的 App 与请求客户端。
func newTestApp(t *testing.T) (*App, *testSSHServer, *httptest.Server) {
	t.Helper()
	srv := newTestSSHServer()
	addr := srv.start(t)
	_, port, _ := net.SplitHostPort(addr)
	portInt := 0
	for _, c := range port {
		portInt = portInt*10 + int(c-'0')
	}

	dir := t.TempDir()
	store, err := configstore.New(dir)
	require.NoError(t, err)
	conn := configstore.Connection{
		Host:       "127.0.0.1",
		Port:       portInt,
		User:       "root",
		Password:   "secret",
		ConfigPath: "/etc/frp/frpc.ini",
		Format:     "ini",
	}
	require.NoError(t, store.Save(conn))

	app := NewApp(store, "")
	mux := http.NewServeMux()
	app.RegisterRoutes(mux)
	hs := httptest.NewServer(mux)
	t.Cleanup(hs.Close)
	return app, srv, hs
}

func doReq(t *testing.T, method, url, body string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	require.NoError(t, err)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(b)
}

// mustStore 用临时目录创建内存持久化存储。
func mustStore(t *testing.T, dir string) configStore {
	t.Helper()
	s, err := configstore.New(dir)
	require.NoError(t, err)
	return s
}

// newMux 为 App 起一个 httptest 服务器。
func newMux(app *App) *httptest.Server {
	mux := http.NewServeMux()
	app.RegisterRoutes(mux)
	hs := httptest.NewServer(mux)
	return hs
}

var _ = context.Background
