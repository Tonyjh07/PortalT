package api

import (
	"errors"
	"net/http"

	"portalt-plugins/frpc-admin/internal/configstore"
)

// handleListConnections GET /api/connections
// 返回全部已保存的连接配置（凭据脱敏）。
func (a *App) handleListConnections(w http.ResponseWriter, _ *http.Request) {
	list := a.store.List()
	out := make([]configstore.Connection, 0, len(list))
	for _, c := range list {
		out = append(out, configstore.Sanitize(c))
	}
	if out == nil {
		out = []configstore.Connection{}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSaveConnection PUT /api/connections/{vmId}
// 保存/更新某 VM 的 SSH 连接与 frpc 管理配置。
// 凭据策略（密码只写不回）：password / sudo_password 为空时沿用已存旧值，
// 允许前端在密码不回显的前提下做部分更新（如仅改路径/端口）。
func (a *App) handleSaveConnection(w http.ResponseWriter, r *http.Request) {
	id := vmID(r)
	var body configstore.Connection
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体必须是 JSON 对象")
		return
	}
	body.VMID = id
	// 凭据沿用旧值：新装场景（旧值不存在）则原样保存（空密码后续 SSH 会失败并报错）。
	if old, ok := a.store.Get(id); ok {
		if body.Password == "" {
			body.Password = old.Password
		}
		if body.SudoPassword == "" {
			body.SudoPassword = old.SudoPassword
		}
	}
	if err := a.store.Save(body); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, configstore.Sanitize(body))
}

// handleDeleteConnection DELETE /api/connections/{vmId}
func (a *App) handleDeleteConnection(w http.ResponseWriter, r *http.Request) {
	id := vmID(r)
	if err := a.store.Delete(id); err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "连接配置不存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"deleted": id})
}
