package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// mustMarshal 将对象序列化为JSON字符串，失败时直接终止测试。
func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
