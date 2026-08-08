package pluginhost

import (
	"encoding/json"

	"portalt/internal/pluginpkg"
)

// manifestJSON 将 manifest 序列化为 JSON 文本（写入 plugins.manifest_json 缓存）。
func manifestJSON(m *pluginpkg.Manifest) (string, error) {
	b, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
