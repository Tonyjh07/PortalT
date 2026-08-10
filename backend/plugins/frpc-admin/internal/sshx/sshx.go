// Package sshx 提供与目标 VM 的 SSH 交互：连接、执行命令、读写文件、重启服务。
//
// 安全约定：
//   - 文件写入通过 stdin 流（cat > 临时文件），不把文件内容拼进 shell 命令；
//   - sudo 场景先写 /tmp 临时文件，再用 `sudo -S`（密码经 stdin 喂入）复制到目标；
//   - 每次操作短连接，不维护长会话；远端 stderr 透出便于排查。
package sshx

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// Config SSH 连接配置（由用户在前端配置）。
type Config struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	User         string `json:"user"`
	Password     string `json:"password"`
	SudoPassword string `json:"sudo_password"` // 空 = 使用 Password 或免密 sudo
	TimeoutSec   int    `json:"timeout_sec"`   // 连接超时，缺省 10
}

// Normalize 填充缺省值并校验必要字段。
func (c *Config) Normalize() error {
	if strings.TrimSpace(c.Host) == "" {
		return errors.New("SSH 主机不能为空")
	}
	if c.Port == 0 {
		c.Port = 22
	}
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("SSH 端口必须在 1-65535")
	}
	if strings.TrimSpace(c.User) == "" {
		return errors.New("SSH 用户名不能为空")
	}
	if c.TimeoutSec <= 0 {
		c.TimeoutSec = 10
	}
	return nil
}

// Conn 一个短生命周期的 SSH 连接。
type Conn struct {
	client *ssh.Client
}

// Dial 建立 SSH 连接（密码认证），带连接超时。
func Dial(cfg Config) (*Conn, error) {
	if err := cfg.Normalize(); err != nil {
		return nil, err
	}
	auth := []ssh.AuthMethod{ssh.Password(cfg.Password)}
	cc := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            auth,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // 本插件面向 homelab，不做固定密钥校验
		Timeout:         time.Duration(cfg.TimeoutSec) * time.Second,
	}
	addr := net.JoinHostPort(cfg.Host, fmt.Sprint(cfg.Port))
	client, err := ssh.Dial("tcp", addr, cc)
	if err != nil {
		return nil, fmt.Errorf("SSH 连接 %s 失败: %w", addr, err)
	}
	return &Conn{client: client}, nil
}

// Close 关闭连接。
func (c *Conn) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}

// RunResult 命令执行结果。
type RunResult struct {
	Stdout string
	Stderr string
	Code   int // 远端退出码（0 = 成功）
}

// Run 执行单条命令；stdin 可选（如 sudo 密码喂入）。
// ctx 控制执行超时：命令结束前 ctx 取消会关闭会话并中断远端命令。
func (c *Conn) Run(ctx context.Context, cmd string, stdin []byte) (RunResult, error) {
	sess, err := c.client.NewSession()
	if err != nil {
		return RunResult{}, fmt.Errorf("创建会话失败: %w", err)
	}
	defer sess.Close()

	var outBuf, errBuf bytes.Buffer
	sess.Stdout = &outBuf
	sess.Stderr = &errBuf
	if len(stdin) > 0 {
		inPipe, err := sess.StdinPipe()
		if err != nil {
			return RunResult{}, fmt.Errorf("获取 stdin 失败: %w", err)
		}
		go func() {
			_, _ = inPipe.Write(stdin)
			_ = inPipe.Close()
		}()
	}

	// 在 goroutine 里执行并等待；ctx 取消时关闭会话中断远端命令，
	// 避免命令挂起导致本调用无限阻塞。
	done := make(chan error, 1)
	go func() {
		done <- sess.Run(cmd)
	}()
	var runErr error
	select {
	case <-ctx.Done():
		_ = sess.Close()
		<-done
		return RunResult{}, fmt.Errorf("命令执行超时: %w", ctx.Err())
	case runErr = <-done:
	}

	res := RunResult{
		Stdout: outBuf.String(),
		Stderr: errBuf.String(),
	}
	var exitErr *ssh.ExitError
	if errors.As(runErr, &exitErr) {
		res.Code = exitErr.ExitStatus()
		return res, nil // 退出码非零不是"调用失败"，由调用方判断
	}
	if runErr != nil {
		return res, fmt.Errorf("执行命令失败: %w", runErr)
	}
	return res, nil
}

// ReadFile 读取远端文件内容（cat）。
func (c *Conn) ReadFile(ctx context.Context, path string) ([]byte, error) {
	res, err := c.Run(ctx, "cat "+quote(path), nil)
	if err != nil {
		return nil, err
	}
	if res.Code != 0 {
		return nil, fmt.Errorf("读取 %s 失败(code=%d): %s", path, res.Code, strings.TrimSpace(res.Stderr))
	}
	return []byte(res.Stdout), nil
}

// WriteFile 写入远端文件。
//
// 流程（保证原子性 + sudo 支持）：
//  1. 写内容到 /tmp/frpc-admin-<rand> 临时文件（stdin 流，不拼 shell），chmod 600；
//  2. sudoPassword 非空时 `sudo -S -p '' cp <tmp> <path>`（密码经 stdin 喂入），
//     否则直接 cp（免密/root 场景，由调用方按 SudoEnabled 决策是否传密码）；
//  3. 清理临时文件。
func (c *Conn) WriteFile(ctx context.Context, path, sudoPassword string, content []byte) error {
	tmp := "/tmp/frpc-admin-" + randomSuffix()
	defer func() {
		_, _ = c.Run(ctx, "rm -f "+quote(tmp), nil)
	}()

	// 1) 写临时文件
	wres, err := c.Run(ctx, "cat > "+quote(tmp), content)
	if err != nil {
		return err
	}
	if wres.Code != 0 {
		return fmt.Errorf("写临时文件失败(code=%d): %s", wres.Code, strings.TrimSpace(wres.Stderr))
	}
	// 临时文件含凭据（token 等），收紧权限（缺省 umask 可能 0644）
	cres0, err := c.Run(ctx, "chmod 600 "+quote(tmp), nil)
	if err != nil {
		return err
	}
	if cres0.Code != 0 {
		return fmt.Errorf("收紧临时文件权限失败(code=%d): %s", cres0.Code, strings.TrimSpace(cres0.Stderr))
	}

	// 2) 复制到目标（必要时 sudo）
	var cpCmd string
	var pass []byte
	if sudoPassword != "" {
		cpCmd = "sudo -S -p '' cp " + quote(tmp) + " " + quote(path)
		pass = []byte(sudoPassword + "\n")
	} else {
		cpCmd = "cp " + quote(tmp) + " " + quote(path)
	}
	cres, err := c.Run(ctx, cpCmd, pass)
	if err != nil {
		return err
	}
	if cres.Code != 0 {
		return fmt.Errorf("写入 %s 失败(code=%d): %s", path, cres.Code, strings.TrimSpace(cres.Stderr))
	}
	return nil
}

// Restart 重启 frpc 服务。restartCmd 为用户配置的重启命令（缺省 systemctl restart frpc）。
// 需要 sudo 时用 sudo -S 包裹并喂密码。返回合并输出。
func (c *Conn) Restart(ctx context.Context, restartCmd, sudoPassword string) (string, error) {
	if strings.TrimSpace(restartCmd) == "" {
		restartCmd = "systemctl restart frpc"
	}
	var cmd string
	var pass []byte
	if sudoPassword != "" {
		cmd = "sudo -S -p '' " + restartCmd
		pass = []byte(sudoPassword + "\n")
	} else {
		cmd = restartCmd
	}
	res, err := c.Run(ctx, cmd, pass)
	if err != nil {
		return "", err
	}
	out := strings.TrimSpace(res.Stdout)
	if e := strings.TrimSpace(res.Stderr); e != "" {
		if out != "" {
			out += "\n"
		}
		out += e
	}
	if res.Code != 0 {
		return out, fmt.Errorf("重启命令失败(code=%d)", res.Code)
	}
	return out, nil
}

// ProbeResult 配置探测结果。
type ProbeResult struct {
	Version    string `json:"version"`
	ConfigPath string `json:"config_path"`
	FormatHint string `json:"format_hint"` // 建议格式（ini/toml/未知）
	Raw        string `json:"raw"`         // 原始探测输出，供用户人工判断
}

// Probe 探测目标 VM 的 frp 版本与配置路径（"怎么看配置路径/格式"的自动化版本）。
func (c *Conn) Probe(ctx context.Context) (ProbeResult, error) {
	var p ProbeResult
	var sb strings.Builder

	steps := []struct{ name, cmd string }{
		{"version", `(which frpc 2>/dev/null && frpc --version 2>&1) || true`},
		{"systemd", `(systemctl cat frpc 2>/dev/null | grep -E 'Exec(Start|Reload|StartPre)') || true`},
		{"ps", `(ps -eo args 2>/dev/null | grep -i 'frpc' | grep -v grep) || true`},
		{"paths", `(ls -l /etc/frp/ /etc/frpc.ini /etc/frpc.toml /usr/local/etc/frpc.ini /usr/local/etc/frpc.toml 2>/dev/null) || true`},
	}
	for _, s := range steps {
		res, err := c.Run(ctx, s.cmd, nil)
		if err != nil {
			return p, err
		}
		text := strings.TrimSpace(res.Stdout)
		if text != "" {
			sb.WriteString("[" + s.name + "]\n" + text + "\n")
		}
		switch s.name {
		case "version":
			// frpc --version 输出形如 "frp x.x.x"，取版本号
			p.Version = strings.TrimSpace(strings.ReplaceAll(strings.Split(text, "\n")[0], "frp ", ""))
		case "systemd", "ps", "paths":
			for _, line := range strings.Split(text, "\n") {
				if c := extractConfigPath(line); c != "" {
					p.ConfigPath = c
					break
				}
			}
		}
	}
	p.Raw = sb.String()
	// 格式提示：优先按配置路径后缀，其次按版本偏好
	if strings.HasSuffix(p.ConfigPath, ".toml") {
		p.FormatHint = "toml"
	} else if strings.HasSuffix(p.ConfigPath, ".ini") || strings.HasSuffix(p.ConfigPath, ".conf") {
		p.FormatHint = "ini"
	} else if p.Version != "" && atLeast052(p.Version) {
		p.FormatHint = "toml"
	}
	return p, nil
}

// quote shell 单引号转义（路径参数不拼入 shell，仍做防御）。
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// randomSuffix 生成随机后缀（临时文件）。
func randomSuffix() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// extractConfigPath 从探测输出行里提取 frpc 配置文件路径（-c <path>）。
func extractConfigPath(line string) string {
	line = strings.TrimSpace(line)
	// ExecStart=/usr/bin/frpc -c /etc/frp/frpc.ini
	if i := strings.Index(line, "-c"); i >= 0 {
		rest := line[i+2:]
		rest = strings.TrimSpace(rest)
		if j := strings.Index(rest, " "); j >= 0 {
			rest = rest[:j]
		}
		rest = strings.Trim(rest, `"'`)
		if rest != "" && strings.Contains(rest, "/") {
			return rest
		}
	}
	// ps 行含 frpc -c <path>
	if strings.Contains(line, "frpc") && strings.Contains(line, " -c ") {
		parts := strings.Fields(line)
		for i, p := range parts {
			if p == "-c" && i+1 < len(parts) {
				return strings.Trim(parts[i+1], `"'`)
			}
		}
	}
	return ""
}

// atLeast052 判断版本号是否 >= 0.52（粗略比较，失败返回 false）。
func atLeast052(v string) bool {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	// 兼容 "0.52.0" / "0.52" / "1.x"
	parts := strings.Split(v, ".")
	if len(parts) == 0 {
		return false
	}
	major, err := parseVer(parts[0])
	if err != nil {
		return false
	}
	if major > 0 {
		return true
	}
	if len(parts) < 2 {
		return false
	}
	minor, err := parseVer(parts[1])
	if err != nil {
		return false
	}
	return minor >= 52
}

func parseVer(s string) (int, error) {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, errors.New("非数字")
		}
		n = n*10 + int(ch-'0')
	}
	return n, nil
}
