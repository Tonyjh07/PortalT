package auth

import (
	"crypto/rand"
	"encoding/hex"
)

// NewID 生成 32 字符随机十六进制ID（加密安全随机源）。
// 供认证层与 API 层（如插件创建）共用。
func NewID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
