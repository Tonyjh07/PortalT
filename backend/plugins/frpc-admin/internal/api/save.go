package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"portalt-plugins/frpc-admin/internal/configstore"
	"portalt-plugins/frpc-admin/internal/frc"
	"portalt-plugins/frpc-admin/internal/sshx"
)

// maxBackups 远端保留的备份份数（超出的旧备份删除）。
const maxBackups = 5

// shQuote shell 单引号转义（备份/回滚命令中的路径参数）。
func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// SaveConfigRequest 保存配置的请求体。content 与 structured 二选一。
type SaveConfigRequest struct {
	Content    string      `json:"content"`    // 原文模式：完整配置文本
	Structured *frc.Config `json:"structured"` // 可视化模式：结构化配置
	Format     string      `json:"format"`     // auto/ini/toml，覆盖检测
}

// SaveConfigResponse 保存流程结果。
type SaveConfigResponse struct {
	SyntaxOK      bool   `json:"syntax_ok"`
	SyntaxError   string `json:"syntax_error,omitempty"`
	BackupPath    string `json:"backup_path,omitempty"`
	Applied       bool   `json:"applied"`
	RestartOutput string `json:"restart_output,omitempty"`
	RolledBack    bool   `json:"rolled_back"`
	RollbackError string `json:"rollback_error,omitempty"`
	Error         string `json:"error,omitempty"` // 前置失败（SSH/读取等）时非空
}

// handlePutConfig PUT /api/vms/{vmId}/config
// 保存 frpc 配置：语法检查 → 备份 → 写入 → 重启 → 失败回滚。
// 全程返回 200 + SaveConfigResponse（应用结果），前置硬失败（无连接配置）返回 4xx。
func (a *App) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	c, ok := a.store.Get(vmID(r))
	if !ok {
		writeErr(w, http.StatusNotFound, "该 VM 尚未配置 SSH 连接，请先在「主机信息」中配置")
		return
	}
	var req SaveConfigRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体必须是 JSON 对象")
		return
	}
	// 请求体必须提供一种内容来源
	if strings.TrimSpace(req.Content) == "" && req.Structured == nil {
		writeErr(w, http.StatusBadRequest, "必须提供 content（原文）或 structured（结构化）")
		return
	}

	res := a.saveConfig(c, req)
	writeJSON(w, http.StatusOK, res)
}

// saveConfig 执行保存流程（SSH 侧操作）。
func (a *App) saveConfig(c configstore.Connection, req SaveConfigRequest) SaveConfigResponse {
	res := SaveConfigResponse{}

	conn, err := dialFor(c)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer conn.Close()
	ctx, cancel := withTimeout()
	defer cancel()

	// 1) 确定路径与格式
	path, err := resolveConfigPath(ctx, conn, c)
	if err != nil {
		res.Error = err.Error()
		return res
	}

	// 2) 构造最终内容与检查格式
	//    原文模式：format 取请求显式值（auto 时由 SyntaxCheck 内部检测）；
	//    结构化模式：以 cfg.Format 为准（若缺省则用请求/连接配置提示），贯穿检查与落盘。
	var content string
	var checkFormat string
	if req.Structured != nil {
		cfg := req.Structured
		if cfg.Format == "" || cfg.Format == frc.FormatAuto {
			cfg.Format = frc.Format(resolveFormat(req.Format, c.Format))
		}
		out, err := cfg.Render()
		if err != nil {
			res.Error = "结构化配置序列化失败：" + err.Error()
			return res
		}
		content = string(out)
		checkFormat = string(cfg.Format)
	} else {
		content = req.Content
		checkFormat = req.Format // 含 "auto"（SyntaxCheck 内部检测）或具体格式
	}

	// 3) 语法检查（解析校验，失败不落盘）
	if err := frc.SyntaxCheck([]byte(content), checkFormat); err != nil {
		res.SyntaxOK = false
		res.SyntaxError = err.Error()
		return res
	}
	res.SyntaxOK = true

	// 4) 备份原文件
	backupPath, err := a.backup(ctx, conn, path, c)
	if err != nil {
		res.Error = "备份原配置失败：" + err.Error()
		return res
	}
	res.BackupPath = backupPath

	sudoPw := a.sudoPasswordFor(c)

	// 5) 写入新配置
	if err := conn.WriteFile(ctx, path, sudoPw, []byte(content)); err != nil {
		// 写入失败：写入是"先临时文件后 cp"，cp 失败时原文件未被改动，
		// 无需回滚（备份保留待人工处理），直接报错。
		res.Applied = false
		res.Error = "写入新配置失败：" + err.Error()
		return res
	}

	// 6) 重启 frpc
	out, err := conn.Restart(ctx, c.RestartCmd, sudoPw)
	if err != nil {
		// 重启失败：回滚配置并再次重启
		res.Applied = false
		res.RestartOutput = out
		a.rollback(ctx, conn, path, backupPath, sudoPw, c.RestartCmd, &res)
		return res
	}
	res.Applied = true
	res.RestartOutput = out
	return res
}

// sudoPasswordFor 返回实际用于 sudo 包裹的密码：SudoEnabled 关闭或目标为 root 时为空。
// SudoEnabled 控制是否经 sudo 执行写文件/重启/备份/回滚（与登录用户名无关）。
func (a *App) sudoPasswordFor(c configstore.Connection) string {
	if !c.SudoEnabled {
		return ""
	}
	if c.SudoPassword != "" {
		return c.SudoPassword
	}
	return c.Password
}

// wrapSudo 按 sudo 场景包裹命令：启用 sudo 时用 sudo -S -p '' <cmd> 并喂密码。
func (a *App) wrapSudo(c configstore.Connection, cmd string) (string, []byte) {
	pw := a.sudoPasswordFor(c)
	if pw == "" {
		return cmd, nil
	}
	return "sudo -S -p '' " + cmd, []byte(pw + "\n")
}

// backup 在远端备份原配置：cp <path> <path>.bak.<ts>，并清理超过 maxBackups 的旧备份。
// 连接启用 sudo 时以 sudo 执行（/etc/frp 等 root 属主目录需要）。
func (a *App) backup(ctx context.Context, conn *sshx.Conn, path string, c configstore.Connection) (string, error) {
	backup := fmt.Sprintf("%s.bak.%d", path, time.Now().Unix())
	cpCmd, pass := a.wrapSudo(c, "cp "+shQuote(path)+" "+shQuote(backup))
	res, err := conn.Run(ctx, cpCmd, pass)
	if err != nil {
		return "", err
	}
	if res.Code != 0 {
		return "", fmt.Errorf("cp 失败(code=%d): %s", res.Code, strings.TrimSpace(res.Stderr))
	}
	// 清理旧备份（保留最新 maxBackups 份）
	prune := fmt.Sprintf(
		`ls -1t %s.bak.* 2>/dev/null | tail -n +%d | xargs -r rm -f`,
		shQuote(path), maxBackups+1)
	pruneCmd, prunePass := a.wrapSudo(c, prune)
	_, _ = conn.Run(ctx, pruneCmd, prunePass)
	return backup, nil
}

// rollback 回滚：cp 备份回原文件 + 再次重启（使用用户配置的重启命令）。
// 结果写入 res。
func (a *App) rollback(ctx context.Context, conn *sshx.Conn, path, backup, sudoPw, restartCmd string, res *SaveConfigResponse) {
	// 恢复备份
	cpCmd, pass := a.sudoCmdFor(sudoPw, "cp "+shQuote(backup)+" "+shQuote(path))
	rres, err := conn.Run(ctx, cpCmd, pass)
	if err != nil || rres.Code != 0 {
		msg := "配置已变更但回滚失败，请人工检查 " + path
		if err != nil {
			msg += "：" + err.Error()
		} else {
			msg += "（code=" + fmt.Sprint(rres.Code) + "）"
		}
		res.RollbackError = msg
		return
	}
	// 回滚后再次重启
	out, err := conn.Restart(ctx, restartCmd, sudoPw)
	if err != nil {
		res.RolledBack = true
		res.RollbackError = "已恢复备份，但 frpc 重启仍失败：" + err.Error() + "；输出：" + out
		return
	}
	res.RolledBack = true
	res.RestartOutput = out
}

// sudoCmdFor 按已有 sudo 密码构造 sudo 包裹命令（rollback 复用调用方算好的密码）。
func (a *App) sudoCmdFor(sudoPw, cmd string) (string, []byte) {
	if sudoPw == "" {
		return cmd, nil
	}
	return "sudo -S -p '' " + cmd, []byte(sudoPw + "\n")
}
