package api

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"portalt-plugins/frpc-admin/internal/configstore"
	"portalt-plugins/frpc-admin/internal/sshx"
)

// backupTSPattern 备份时间戳严格约束（纯数字），同时防路径穿越。
var backupTSPattern = regexp.MustCompile(`^[0-9]+$`)

// BackupInfo 一条远端备份信息。
type BackupInfo struct {
	Path string `json:"path"`
	TS   string `json:"ts"`
	Size int64  `json:"size"`
}

// ListBackupsResponse 备份列表响应。
type ListBackupsResponse struct {
	Backups []BackupInfo `json:"backups"`
}

// handleListBackups GET /api/backups
// 列出远端 frpc 配置的备份（<path>.bak.<ts>），按时间戳倒序。
// 用 stat 一次拿全「大小 路径」，Go 侧解析并排序，避免多次往返。
func (a *App) handleListBackups(w http.ResponseWriter, r *http.Request) {
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
	cmd, pass := a.wrapSudo(c, statGlobCmd(path))
	res, err := conn.Run(ctx, cmd, pass)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "SSH 执行失败："+err.Error())
		return
	}
	// 无匹配文件时 stat 报错（code!=0 + 空 stdout），视为"暂无备份"
	if res.Code != 0 && strings.TrimSpace(res.Stdout) == "" {
		writeJSON(w, http.StatusOK, ListBackupsResponse{Backups: []BackupInfo{}})
		return
	}
	backups := parseBackupStat(res.Stdout)
	// 时间戳按数值倒序（字符串比较在位数不同时会错排，如 999999999 < 1000000000）
	sort.Slice(backups, func(i, j int) bool {
		ti, _ := strconv.ParseInt(backups[i].TS, 10, 64)
		tj, _ := strconv.ParseInt(backups[j].TS, 10, 64)
		return ti > tj
	})
	writeJSON(w, http.StatusOK, ListBackupsResponse{Backups: backups})
}

// BackupContentResponse 单个备份内容响应。
type BackupContentResponse struct {
	Path    string `json:"path"`
	TS      string `json:"ts"`
	Size    int64  `json:"size"`
	Content string `json:"content"`
}

// handleGetBackup GET /api/backups/{ts}
// 查看指定备份内容。ts 必须为纯数字（服务端拼路径，防穿越）。
func (a *App) handleGetBackup(w http.ResponseWriter, r *http.Request) {
	ts, err := backupTS(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
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
	backup := backupPathFor(path, ts)
	content, size, err := a.readBackup(ctx, conn, c, backup)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, BackupContentResponse{
		Path: backup, TS: ts, Size: size, Content: content,
	})
}

// handleRestoreBackup POST /api/backups/{ts}/restore
// 恢复指定备份：先备份当前配置（保证可回退）→ 覆盖原配置 → 重启 → 失败回滚。
// 返回体复用 SaveConfigResponse（BackupPath 为恢复前的"当前配置备份"）。
func (a *App) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	ts, err := backupTS(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	c, ok := a.store.Get()
	if !ok {
		writeErr(w, http.StatusNotFound, "尚未配置 SSH 连接")
		return
	}
	res := a.restoreConfig(c, ts)
	writeJSON(w, http.StatusOK, res)
}

// restoreConfig 执行恢复流程（SSH 侧）。核心与 saveConfig 对齐：
// 备份当前 → 覆盖写入 → 重启 → 失败回滚。
// 目标备份路径由服务端拼接（ts 已在上层校验为纯数字），
// 恢复前确保该备份存在且可读。
func (a *App) restoreConfig(c configstore.Connection, ts string) SaveConfigResponse {
	res := SaveConfigResponse{}

	conn, err := dialFor(c)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer conn.Close()
	ctx, cancel := withTimeout()
	defer cancel()

	path, err := resolveConfigPath(ctx, conn, c)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	backup := backupPathFor(path, ts)

	// 恢复前校验备份存在（cat 失败视为不存在）
	if _, _, err := a.readBackup(ctx, conn, c, backup); err != nil {
		res.Error = "目标备份不存在：" + err.Error()
		return res
	}

	// 1) 备份当前配置（恢复可回退）
	curBackup, err := a.backup(ctx, conn, path, c)
	if err != nil {
		res.Error = "备份当前配置失败：" + err.Error()
		return res
	}
	res.BackupPath = curBackup

	sudoPw := a.sudoPasswordFor(c)

	// 2) 覆盖写入目标备份内容
	//    cp 为"先截断后写"非原子：失败时可能残留半写配置，故与保存链路一致，
	//    尝试回滚到 curBackup（恢复前的当前配置），保证状态可恢复。
	cpCmd, pass := a.sudoCmdFor(sudoPw, "cp "+shQuote(backup)+" "+shQuote(path))
	cres, err := conn.Run(ctx, cpCmd, pass)
	if err != nil {
		res.Error = "恢复备份失败：" + err.Error()
		a.rollback(ctx, conn, path, curBackup, sudoPw, c.RestartCmd, &res)
		return res
	}
	if cres.Code != 0 {
		msg := fmt.Sprintf("恢复备份失败(code=%d): %s", cres.Code, strings.TrimSpace(cres.Stderr))
		res.Error = msg
		a.rollback(ctx, conn, path, curBackup, sudoPw, c.RestartCmd, &res)
		return res
	}

	// 3) 重启
	out, err := conn.Restart(ctx, c.RestartCmd, sudoPw)
	if err != nil {
		res.Applied = false
		res.RestartOutput = out
		a.rollback(ctx, conn, path, curBackup, sudoPw, c.RestartCmd, &res)
		return res
	}
	res.Applied = true
	res.RestartOutput = out
	return res
}

// backupTS 从请求路径提取备份时间戳并校验格式。
func backupTS(r *http.Request) (string, error) {
	ts := r.PathValue("ts")
	if !backupTSPattern.MatchString(ts) {
		return "", fmt.Errorf("备份标识必须是纯数字（当前值 %q）", ts)
	}
	return ts, nil
}

// backupPathFor 由配置路径与时间戳拼接备份路径（服务端拼装，防穿越）。
func backupPathFor(path, ts string) string {
	return fmt.Sprintf("%s.bak.%s", path, ts)
}

// statGlobCmd 列出全部备份的「路径 大小」：stat -c '%n %s' <path>.bak.*。
// 无匹配时 stat 非零退出，由调用方判空。
func statGlobCmd(path string) string {
	return "stat -c '%n %s' " + shQuote(path) + ".bak.*"
}

// readBackup 读取备份内容与大小（cat）。备份必须存在（code==0）。
// 不依赖 stat：以 cat 输出长度为大小，减少一次往返。
func (a *App) readBackup(ctx context.Context, conn *sshx.Conn, c configstore.Connection, backup string) (string, int64, error) {
	cmd, pass := a.wrapSudo(c, "cat "+shQuote(backup))
	res, err := conn.Run(ctx, cmd, pass)
	if err != nil {
		return "", 0, err
	}
	if res.Code != 0 {
		return "", 0, fmt.Errorf("读取 %s 失败(code=%d): %s", backup, res.Code, strings.TrimSpace(res.Stderr))
	}
	return res.Stdout, int64(len(res.Stdout)), nil
}

// parseBackupStat 解析 stat 输出（每行「路径 大小」），只保留 .bak.<ts> 备份文件。
// 路径可能含空格，故按最后一个空格切分（大小为纯数字，安全）。
// 时间戳非纯数字的（如人工命名的 .bak.xxx）会作为非备份文件名被忽略。
func parseBackupStat(out string) []BackupInfo {
	list := []BackupInfo{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		i := strings.LastIndex(line, " ")
		if i < 0 {
			continue
		}
		pathPart := strings.TrimSpace(line[:i])
		sizePart := strings.TrimSpace(line[i+1:])
		ts, ok := backupTSFromPath(pathPart)
		if !ok {
			continue
		}
		size, err := strconv.ParseInt(sizePart, 10, 64)
		if err != nil {
			continue
		}
		list = append(list, BackupInfo{Path: pathPart, TS: ts, Size: size})
	}
	return list
}

// backupTSFromPath 从备份路径提取时间戳后缀（<path>.bak.<ts>）。
func backupTSFromPath(p string) (string, bool) {
	idx := strings.LastIndex(p, ".bak.")
	if idx < 0 {
		return "", false
	}
	ts := p[idx+len(".bak."):]
	if !backupTSPattern.MatchString(ts) {
		return "", false
	}
	return ts, true
}