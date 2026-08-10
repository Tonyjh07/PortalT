package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// decodeJSON 读取并解析请求体 JSON。拒绝尾部多余数据（防错配字段/多对象混淆）。
func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, 4<<20))
	if err := dec.Decode(v); err != nil {
		return err
	}
	// 再读一次：应到达 EOF，否则视为非法请求体
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("请求体必须是单个 JSON 对象")
	}
	return nil
}

// writeJSON 写 JSON 响应。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr 写错误响应：{ "error": "..." }。
func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// writeErrWithFields 写错误响应并携带附加字段（供前端展示）。
func writeErrWithFields(w http.ResponseWriter, status int, msg string, fields map[string]any) {
	body := map[string]any{"error": msg}
	for k, v := range fields {
		body[k] = v
	}
	writeJSON(w, status, body)
}
