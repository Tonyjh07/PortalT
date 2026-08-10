package api

import (
	"errors"
	"net/http"

	"portalt-plugins/frpc-admin/internal/configstore"
)

// handleGetConnection GET /api/connection
// 返回已保存的连接配置（凭据脱敏）；未配置返回 404。
func (a *App) handleGetConnection(w http.ResponseWriter, _ *http.Request) {
	c, ok := a.store.Get()
	if !ok {
		writeErr(w, http.StatusNotFound, "尚未配置 SSH 连接")
		return
	}
	writeJSON(w, http.StatusOK, configstore.Sanitize(c))
}

// handleSaveConnection PUT /api/connection
// 保存/更新 SSH 连接与 frpc 管理配置。
// 凭据策略（密码只写不回）：password / sudo_password 为空时沿用已存旧值，
// 允许前端在密码不回显的前提下做部分更新（如仅改路径/端口）。
func (a *App) handleSaveConnection(w http.ResponseWriter, r *http.Request) {
	var body configstore.Connection
	if err := decodeJSON(r, &body); err != nil {
		writeErr(w, http.StatusBadRequest, "请求体必须是 JSON 对象")
		return
	}
	// 凭据沿用旧值：新装场景（旧值不存在）则原样保存（空密码后续 SSH 会失败并报错）。
	if old, ok := a.store.Get(); ok {
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

// handleDeleteConnection DELETE /api/connection
func (a *App) handleDeleteConnection(w http.ResponseWriter, _ *http.Request) {
	if err := a.store.Delete(); err != nil {
		if errors.Is(err, configstore.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "连接配置不存在")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"deleted": "true"})
}
