package configstore

import (
	"bytes"
	"encoding/json"
)

// trimBytes 去除首尾空白（避免把空白 JSON 当数据）。
func trimBytes(b []byte) []byte {
	return bytes.TrimSpace(b)
}

// migrateLegacy 兼容旧版多连接格式（map[vm_id]Connection）：
// 取排序后的第一条作为当前单连接返回；无数据时返回 nil。
func migrateLegacy(b []byte) *Connection {
	var m map[string]Connection
	if err := json.Unmarshal(b, &m); err != nil || len(m) == 0 {
		return nil
	}
	// 取键最小的那条（确定性）
	var key string
	var first Connection
	found := false
	for k := range m {
		if !found || k < key {
			key = k
			first = m[k]
			found = true
		}
	}
	if !found {
		return nil
	}
	return &first
}
