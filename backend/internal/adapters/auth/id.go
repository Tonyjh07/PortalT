package auth

import (
	"crypto/rand"
	"encoding/hex"
)

// newID 生成 32 字符随机十六进制ID（加密安全随机源）。
func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
