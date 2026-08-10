package sshx

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/crypto/ssh"
)

// fakeServer 脚本化假 SSH 服务器：按命令字符串返回预设输出，模拟文件系统。
// 验证客户端密码认证、exec、stdin 喂入、退出码、文件读写、sudo、重启全链路。
type fakeServer struct {
	t            *testing.T
	ln           net.Listener
	password     string
	mu           sync.Mutex
	files        map[string]string // path -> content（含 /tmp 临时文件）
	restart      []string          // 记录重启命令调用
	sudoPassword string            // 最近一次 sudo -S 收到的密码
	exitCode     int               // 最近一次命令退出码
}

func newFakeServer(t *testing.T, password string) *fakeServer {
	t.Helper()
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(_ ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if string(pass) == password {
				return nil, nil
			}
			return nil, fmt.Errorf("密码错误")
		},
	}
	key := testSigner(t)
	cfg.AddHostKey(key)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	s := &fakeServer{t: t, ln: ln, password: password, files: map[string]string{}}
	go s.serve(cfg)
	return s
}

// testSigner 生成 ed25519 主机密钥。
func testSigner(t *testing.T) ssh.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(priv)
	require.NoError(t, err)
	return signer
}

func (s *fakeServer) addr() string { return s.ln.Addr().String() }

func (s *fakeServer) close() { _ = s.ln.Close() }

func (s *fakeServer) setFile(path, content string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[path] = content
}

func (s *fakeServer) getFile(path string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.files[path]
	return v, ok
}

func (s *fakeServer) restartCalls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.restart...)
}

func (s *fakeServer) serve(cfg *ssh.ServerConfig) {
	for {
		conn, err := s.ln.Accept()
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
					_ = newCh.Reject(ssh.UnknownChannelType, "仅支持 session")
					continue
				}
				ch, chReqs, err := newCh.Accept()
				if err != nil {
					continue
				}
				go s.handleChannel(srvConn, ch, chReqs)
			}
		}(conn)
	}
}

func (s *fakeServer) handleChannel(_ *ssh.ServerConn, ch ssh.Channel, reqs <-chan *ssh.Request) {
	defer ch.Close()
	for req := range reqs {
		switch req.Type {
		case "exec":
			var payload struct{ Command string }
			_ = ssh.Unmarshal(req.Payload, &payload)
			if !req.WantReply {
				continue
			}
			_ = req.Reply(true, nil)
			s.exec(ch, payload.Command)
			_, _ = ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(s.exitCode)}))
			return
		case "pty-req":
			_ = req.Reply(true, nil)
		case "shell":
			_ = req.Reply(false, nil)
		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
	}
}

// exec 分发命令。命令由客户端构造，固定前缀匹配。
func (s *fakeServer) exec(ch ssh.Channel, cmd string) {
	cmd = strings.TrimSpace(cmd)
	switch {
	case strings.HasPrefix(cmd, "cat > "):
		path := stripQuote(strings.TrimPrefix(cmd, "cat > "))
		content, _ := io.ReadAll(ch)
		s.setFile(path, string(content))
		s.exitCode = 0
	case strings.HasPrefix(cmd, "chmod "):
		s.exitCode = 0
	case strings.HasPrefix(cmd, "cat "):
		path := stripQuote(strings.TrimPrefix(cmd, "cat "))
		content, ok := s.getFile(path)
		if !ok {
			_, _ = io.WriteString(ch, "cat: "+path+": No such file or directory\n")
			s.exitCode = 1
			return
		}
		_, _ = io.WriteString(ch, content)
		s.exitCode = 0
	case strings.HasPrefix(cmd, "sudo -S"):
		buf, _ := readLine(ch)
		s.mu.Lock()
		s.sudoPassword = strings.TrimSpace(string(buf))
		s.mu.Unlock()
		rest := strings.TrimPrefix(cmd, "sudo -S")
		// 剥掉 `-p '' ` 前缀
		if i := strings.Index(rest, "'' "); i >= 0 {
			rest = rest[i+3:]
		}
		s.exec(ch, strings.TrimSpace(rest))
	case strings.HasPrefix(cmd, "cp "):
		parts := fieldsQuote(cmd)
		// cp <tmp> <path>
		if len(parts) == 3 {
			content, ok := s.getFile(stripQuote(parts[1]))
			if ok {
				s.setFile(stripQuote(parts[2]), content)
			}
		}
		s.exitCode = 0
	case strings.HasPrefix(cmd, "rm -f "):
		path := stripQuote(strings.TrimPrefix(cmd, "rm -f "))
		s.mu.Lock()
		delete(s.files, path)
		s.mu.Unlock()
		s.exitCode = 0
	case strings.HasPrefix(cmd, "systemctl restart"):
		s.mu.Lock()
		s.restart = append(s.restart, cmd)
		s.mu.Unlock()
		_, _ = io.WriteString(ch, "Restarting frpc... ok\n")
		s.exitCode = 0
	case strings.HasPrefix(cmd, "restart-frpc"):
		s.mu.Lock()
		s.restart = append(s.restart, cmd)
		s.mu.Unlock()
		_, _ = io.WriteString(ch, "custom restart ok\n")
		s.exitCode = 0
	case strings.Contains(cmd, "frpc --version"):
		_, _ = io.WriteString(ch, "frp 0.54.0\n")
		s.exitCode = 0
	case strings.Contains(cmd, "systemctl cat frpc"):
		_, _ = io.WriteString(ch, "ExecStart=/usr/bin/frpc -c /etc/frp/frpc.toml\n")
		s.exitCode = 0
	case strings.Contains(cmd, "ps -eo"):
		_, _ = io.WriteString(ch, "frpc -c /etc/frp/frpc.toml\n")
		s.exitCode = 0
	case strings.Contains(cmd, "ls -l"):
		_, _ = io.WriteString(ch, "/etc/frp/frpc.toml\n")
		s.exitCode = 0
	default:
		_, _ = io.WriteString(ch, "command not found\n")
		s.exitCode = 127
	}
}

func portOf(addr string) int {
	parts := strings.Split(addr, ":")
	var port int
	_, _ = fmt.Sscanf(parts[len(parts)-1], "%d", &port)
	return port
}

func stripQuote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1]
	}
	return s
}

// fieldsQuote 简易按空格切分（引号内空格不拆）。
func fieldsQuote(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '\'':
			inQuote = !inQuote
		case r == ' ' && !inQuote:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func readLine(r io.Reader) ([]byte, error) {
	var buf bytes.Buffer
	one := make([]byte, 1)
	for {
		n, err := r.Read(one)
		if n > 0 {
			buf.Write(one[:n])
			if one[0] == '\n' {
				return buf.Bytes(), nil
			}
		}
		if err != nil {
			return buf.Bytes(), err
		}
	}
}

// ---- 测试用例 ----

func TestDialBadPassword(t *testing.T) {
	srv := newFakeServer(t, "secret")
	defer srv.close()
	_, err := Dial(Config{Host: "127.0.0.1", Port: portOf(srv.addr()), User: "root", Password: "wrong"})
	assert.Error(t, err)
}

func TestReadFile(t *testing.T) {
	srv := newFakeServer(t, "secret")
	defer srv.close()
	srv.setFile("/etc/frp/frpc.toml", "serverAddr = \"1.2.3.4\"\n")

	conn, err := Dial(Config{Host: "127.0.0.1", Port: portOf(srv.addr()), User: "root", Password: "secret"})
	require.NoError(t, err)
	defer conn.Close()

	content, err := conn.ReadFile(context.Background(), "/etc/frp/frpc.toml")
	require.NoError(t, err)
	assert.Equal(t, "serverAddr = \"1.2.3.4\"\n", string(content))
}

func TestReadFileNotFound(t *testing.T) {
	srv := newFakeServer(t, "secret")
	defer srv.close()
	conn, err := Dial(Config{Host: "127.0.0.1", Port: portOf(srv.addr()), User: "root", Password: "secret"})
	require.NoError(t, err)
	defer conn.Close()

	_, err = conn.ReadFile(context.Background(), "/nonexistent")
	assert.Error(t, err)
}

func TestWriteFileNoSudo(t *testing.T) {
	srv := newFakeServer(t, "secret")
	defer srv.close()
	conn, err := Dial(Config{Host: "127.0.0.1", Port: portOf(srv.addr()), User: "root", Password: "secret"})
	require.NoError(t, err)
	defer conn.Close()

	err = conn.WriteFile(context.Background(), "/etc/frp/frpc.ini", "", []byte("server_addr = 1.2.3.4\n"))
	require.NoError(t, err)
	content, ok := srv.getFile("/etc/frp/frpc.ini")
	require.True(t, ok)
	assert.Equal(t, "server_addr = 1.2.3.4\n", content)
}

func TestWriteFileWithSudo(t *testing.T) {
	srv := newFakeServer(t, "secret")
	defer srv.close()
	conn, err := Dial(Config{Host: "127.0.0.1", Port: portOf(srv.addr()), User: "root", Password: "secret"})
	require.NoError(t, err)
	defer conn.Close()

	err = conn.WriteFile(context.Background(), "/etc/frp/frpc.toml", "sudopass", []byte("serverPort = 7000\n"))
	require.NoError(t, err)
	content, ok := srv.getFile("/etc/frp/frpc.toml")
	require.True(t, ok)
	assert.Equal(t, "serverPort = 7000\n", content)

	srv.mu.Lock()
	sudoGot := srv.sudoPassword
	srv.mu.Unlock()
	assert.Equal(t, "sudopass", sudoGot, "sudo 密码应经 stdin 喂入")
}

func TestRestartDefaultAndCustom(t *testing.T) {
	srv := newFakeServer(t, "secret")
	defer srv.close()
	conn, err := Dial(Config{Host: "127.0.0.1", Port: portOf(srv.addr()), User: "root", Password: "secret"})
	require.NoError(t, err)
	defer conn.Close()

	out, err := conn.Restart(context.Background(), "", "sudopass")
	require.NoError(t, err)
	assert.Contains(t, out, "Restarting frpc")

	out2, err := conn.Restart(context.Background(), "restart-frpc", "")
	require.NoError(t, err)
	assert.Contains(t, out2, "custom restart")
	assert.Len(t, srv.restartCalls(), 2)
}

func TestProbe(t *testing.T) {
	srv := newFakeServer(t, "secret")
	defer srv.close()
	srv.setFile("/etc/frp/frpc.toml", "serverAddr = \"1.2.3.4\"\n")
	conn, err := Dial(Config{Host: "127.0.0.1", Port: portOf(srv.addr()), User: "root", Password: "secret"})
	require.NoError(t, err)
	defer conn.Close()

	p, err := conn.Probe(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "0.54.0", p.Version)
	assert.Equal(t, "/etc/frp/frpc.toml", p.ConfigPath)
	assert.Equal(t, "toml", p.FormatHint)
}

func TestNormalize(t *testing.T) {
	c := Config{Host: "   ", User: "root"}
	assert.Error(t, c.Normalize())

	c = Config{Host: "10.0.0.1", User: "root"}
	require.NoError(t, c.Normalize())
	assert.Equal(t, 22, c.Port)
	assert.Equal(t, 10, c.TimeoutSec)

	c = Config{Host: "10.0.0.1", Port: 99999}
	assert.Error(t, c.Normalize())
}
