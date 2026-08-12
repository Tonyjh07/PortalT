package api

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"portalt-plugins/frpc-admin/internal/configstore"
	"portalt-plugins/frpc-admin/internal/frc"
	"portalt-plugins/frpc-admin/internal/sshx"
)

// defaultLogLines 日志默认行数。
const defaultLogLines = 200

// minLogLines / maxLogLines 日志行数边界（防一次拉取过大/过小无意义）。
const (
	minLogLines = 50
	maxLogLines = 2000
)

// journalUnitRe 从重启命令提取 systemd unit 名（systemctl restart <unit>，容忍
// sudo 前缀与系统级 --flag）。unit 字符集限制避免抓错下一个 token。
var journalUnitRe = regexp.MustCompile(`(?:^|\s)(?:sudo\s+)?systemctl(?:\s+--[^\s]+)*\s+restart\s+([A-Za-z0-9_.@-]+)`)

// LogsResponse 日志读取响应。
type LogsResponse struct {
	Source  string `json:"source"` // "journal" | "file"
	Path    string `json:"path"`   // journal 时为 unit，file 时为文件路径
	Lines   int    `json:"lines"`
	Content string `json:"content,omitempty"`
	Error   string `json:"error,omitempty"` // 读取失败原因（SSH/命令失败等）
}

// handleGetLogs GET /api/logs?lines=200
// 读取远端 frpc 日志。来源自动检测：配置文件声明了真实日志路径
// （TOML log.to / INI log_file）→ tail <path>；否则 journalctl -u <unit>，
// unit 默认 frpc、可从重启命令（systemctl restart <unit>）自动提取。
// 按连接 sudo 配置包裹命令。
func (a *App) handleGetLogs(w http.ResponseWriter, r *http.Request) {
	c, ok := a.store.Get()
	if !ok {
		writeErr(w, http.StatusNotFound, "尚未配置 SSH 连接")
		return
	}
	lines := defaultLogLines
	if v := r.URL.Query().Get("lines"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			lines = n
		}
	}
	lines = clamp(lines, minLogLines, maxLogLines)

	conn, err := dialFor(c)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	defer conn.Close()

	ctx, cancel := withTimeout()
	defer cancel()

	src := a.detectLogSource(ctx, conn, c)
	cmd := logCommandFor(src, lines)
	runCmd, pass := a.wrapSudo(c, cmd)
	res, err := conn.Run(ctx, runCmd, pass)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "SSH 执行失败："+err.Error())
		return
	}
	if res.Code != 0 {
		writeJSON(w, http.StatusOK, LogsResponse{
			Source: src.Mode, Path: src.Path, Lines: lines,
			Error: fmt.Sprintf("日志命令失败(code=%d): %s", res.Code, strings.TrimSpace(res.Stderr)),
		})
		return
	}
	writeJSON(w, http.StatusOK, LogsResponse{
		Source: src.Mode, Path: src.Path, Lines: lines,
		Content: strings.TrimSpace(res.Stdout),
	})
}

// logSource 日志来源。
type logSource struct {
	Mode string // "journal" | "file"
	Path string // journal: unit；file: 文件路径
}

// detectLogSource 自动检测日志来源。
// 主路径：读取远端配置文件，若声明真实日志路径走 tail 文件；探测失败/未声明走 journal。
// 解析失败（连接格式提示与实际不符）时回退 frc.Detect 自动检测，避免误降 journal。
func (a *App) detectLogSource(ctx context.Context, conn *sshx.Conn, c configstore.Connection) logSource {
	path, err := resolveConfigPath(ctx, conn, c)
	if err != nil {
		// 无配置路径：退回 journal（unit 从重启命令提取，默认 frpc）
		return logSource{Mode: "journal", Path: systemdUnitFrom(c.RestartCmd)}
	}
	content, err := conn.ReadFile(ctx, path)
	if err != nil {
		return logSource{Mode: "journal", Path: systemdUnitFrom(c.RestartCmd)}
	}
	// 先用连接格式提示解析；只要从中取到真实日志路径即用。
	// 取不到（格式提示不符/解析失败/未声明路径）时按内容自动检测再试一次
	// （如 ini 提示实际 TOML；检测消费 Detect 结果与 handleGetConfig 一致）。
	fmts := []string{resolveFormat(c.Format, "")}
	if d := string(frc.Detect(content)); d != fmts[0] {
		fmts = append(fmts, d)
	}
	for _, f := range fmts {
		parsed, perr := frc.Parse(content, f)
		if perr != nil {
			continue
		}
		if lp := logFilePath(parsed); lp != "" {
			return logSource{Mode: "file", Path: lp}
		}
	}
	return logSource{Mode: "journal", Path: systemdUnitFrom(c.RestartCmd)}
}

// logFilePath 从解析后的配置提取真实日志文件路径；console/stderr/缺失视为无。
// TOML：[log].to（frp >= 0.52）；INI：[common].log_file（frp < 0.52）。
func logFilePath(cfg *frc.Config) string {
	if cfg == nil {
		return ""
	}
	if cfg.Format == frc.FormatTOML {
		// [log] 表落在 Server.Extra 的 "log" 键（map[string]any）
		if lg, ok := cfg.Server.Extra["log"].(map[string]any); ok {
			if to, ok := lg["to"].(string); ok {
				t := strings.TrimSpace(to)
				t = strings.TrimPrefix(t, "file://")
				if t != "" && t != "console" && t != "stderr" {
					return t
				}
			}
		}
		return ""
	}
	// INI：[common].log_file
	if v, ok := cfg.Server.Extra["log_file"].(string); ok {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}

// systemdUnitFrom 从重启命令提取 systemd unit 名；无法识别时回退 "frpc"。
func systemdUnitFrom(restartCmd string) string {
	if m := journalUnitRe.FindStringSubmatch(restartCmd); len(m) == 2 && m[1] != "" {
		return m[1]
	}
	return "frpc"
}

// logCommandFor 构造日志读取命令（路径/unit 均经 shell 单引号转义）。
func logCommandFor(src logSource, lines int) string {
	n := strconv.Itoa(lines)
	if src.Mode == "file" {
		return "tail -n " + n + " " + shQuote(src.Path)
	}
	return "journalctl -u " + shQuote(src.Path) + " -n " + n + " --no-pager"
}

// clamp 钳制 n 到 [lo, hi]。
func clamp(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}