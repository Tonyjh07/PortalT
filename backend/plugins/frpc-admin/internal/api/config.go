package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"portalt-plugins/frpc-admin/internal/configstore"
	"portalt-plugins/frpc-admin/internal/frc"
	"portalt-plugins/frpc-admin/internal/sshx"
)

// opTimeout 单次 SSH 操作的会话超时（读/写/重启/探测）。
const opTimeout = 30 * time.Second

// withTimeout 构造带超时的上下文。
func withTimeout() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), opTimeout)
}

// dialFor 按连接配置建立 SSH 连接（统一错误处理）。
func dialFor(c configstore.Connection) (*sshx.Conn, error) {
	cfg := sshx.Config{
		Host:         c.Host,
		Port:         c.Port,
		User:         c.User,
		Password:     c.Password,
		SudoPassword: c.SudoPassword,
	}
	conn, err := sshx.Dial(cfg)
	if err != nil {
		return nil, fmt.Errorf("SSH 连接失败: %w", err)
	}
	return conn, nil
}

// resolveConfigPath 确定 frpc 配置文件路径：连接配置显式值优先，否则探测。
func resolveConfigPath(ctx context.Context, conn *sshx.Conn, c configstore.Connection) (string, error) {
	if strings.TrimSpace(c.ConfigPath) != "" {
		return c.ConfigPath, nil
	}
	p, err := conn.Probe(ctx)
	if err != nil {
		return "", err
	}
	if p.ConfigPath == "" {
		return "", errors.New("未配置 frpc 配置路径，且自动探测未识别到；请在「主机信息」中填写，或用「检测」查看")
	}
	return p.ConfigPath, nil
}

// resolveFormat 确定配置格式：请求显式值 > 连接配置 > 自动检测（auto）。
// 返回 "auto" 时由调用方（frc.Parse / frc.Detect）自动检测具体格式。
// 注意：旧实现把未识别的值（如 "auto"）直接回退为 ini，导致 TOML 配置被 INI
// 解析器错误解析（代理列表只剩最后一个 [[proxies]] 项）——已改为回退到自动检测。
func resolveFormat(f string, hint string) string {
	if frc.ValidateFormat(f) {
		return f
	}
	if frc.ValidateFormat(hint) {
		return hint
	}
	return string(frc.FormatAuto)
}

// handleProbe POST /api/probe
// SSH 探测 frp 版本与配置路径（"怎么看配置路径/格式"的自动化）。
// 针对已保存连接配置。
func (a *App) handleProbe(w http.ResponseWriter, r *http.Request) {
	c, ok := a.store.Get()
	if !ok {
		writeErr(w, http.StatusNotFound, "尚未配置 SSH 连接")
		return
	}
	conn, err := dialFor(c)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	defer conn.Close()

	ctx, cancel := withTimeout()
	defer cancel()
	p, err := conn.Probe(ctx)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// ConfigResponse 读取配置的响应体。
type ConfigResponse struct {
	Content string       `json:"content"` // 配置原文
	Format  string       `json:"format"`
	Server  any          `json:"server"`
	Proxies []frc.Proxy  `json:"proxies"`
	Path    string       `json:"path"`
}

// handleGetConfig GET /api/config
// SSH 读取 frpc 配置并解析为结构化数据（可视化编辑 + 原文编辑共用）。
func (a *App) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	c, ok := a.store.Get()
	if !ok {
		writeErr(w, http.StatusNotFound, "尚未配置 SSH 连接")
		return
	}
	conn, err := dialFor(c)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	defer conn.Close()

	ctx, cancel := withTimeout()
	defer cancel()

	path, err := resolveConfigPath(ctx, conn, c)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	content, err := conn.ReadFile(ctx, path)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "读取配置文件失败（文件可能不存在）："+err.Error())
		return
	}
	format := resolveFormat(c.Format, "")
	parsed, err := frc.Parse(content, format)
	if err != nil {
		// 原文件无法解析：仍返回原文供文本编辑/人工修复
		writeErrWithFields(w, http.StatusUnprocessableEntity, "配置文件解析失败："+err.Error(), map[string]any{
			"content": string(content),
			"format":  frc.Detect(content),
			"path":    path,
		})
		return
	}
	writeJSON(w, http.StatusOK, ConfigResponse{
		Content: string(content),
		Format:  string(parsed.Format),
		Server:  parsed.Server,
		Proxies: parsed.Proxies,
		Path:    path,
	})
}
