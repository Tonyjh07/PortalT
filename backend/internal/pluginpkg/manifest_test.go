package pluginpkg

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeManifest 把内容写入临时目录并返回文件路径。
func writeManifest(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestLoad_Valid(t *testing.T) {
	path := writeManifest(t, `{
		"id": "demo",
		"name": "演示插件",
		"icon": "mdi:puzzle",
		"route": "/demo",
		"sort_order": 50,
		"permission": "demo:use",
		"health_interval_seconds": 15
	}`)
	m, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "demo", m.ID)
	assert.Equal(t, "演示插件", m.Name)
	assert.Equal(t, "mdi:puzzle", m.Icon)
	assert.Equal(t, "/demo", m.Route)
	assert.Equal(t, 50, m.SortOrder)
	assert.Equal(t, "demo:use", m.Permission)
	assert.Equal(t, 15, m.HealthInterval())
}

func TestLoad_Minimal(t *testing.T) {
	// 仅必填字段，未声明字段用默认值。
	path := writeManifest(t, `{"id": "demo", "name": "演示", "route": "/demo"}`)
	m, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "demo", m.ID)
	assert.Equal(t, 0, m.SortOrder)
	assert.Equal(t, "", m.Permission)
	// 健康间隔未声明 → 默认值
	assert.Equal(t, DefaultHealthInterval, m.HealthInterval())
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.json"))
	require.Error(t, err)
	assert.ErrorContains(t, err, "读取 manifest")
}

func TestLoad_InvalidJSON(t *testing.T) {
	path := writeManifest(t, `{invalid json`)
	_, err := Load(path)
	require.Error(t, err)
	assert.ErrorContains(t, err, "解析 manifest")
}

func TestValidate_Errors(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"空 ID", `{"id": "", "name": "n", "route": "/a"}`, "ID 不能为空"},
		{"空名称", `{"id": "a", "name": "", "route": "/a"}`, "名称不能为空"},
		{"空路由", `{"id": "a", "name": "n", "route": ""}`, "路由不能为空"},
		{"路由无前导斜杠", `{"id": "a", "name": "n", "route": "a"}`, "以 / 开头"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := writeManifest(t, tc.content)
			_, err := Load(path)
			require.Error(t, err)
			assert.ErrorContains(t, err, tc.want)
		})
	}
}

func TestHealthInterval_InvalidUsesDefault(t *testing.T) {
	m := &Manifest{HealthIntervalSeconds: -5}
	assert.Equal(t, DefaultHealthInterval, m.HealthInterval())
	m = &Manifest{}
	assert.Equal(t, DefaultHealthInterval, m.HealthInterval())
	assert.Equal(t, DefaultHealthInterval, (*Manifest)(nil).HealthInterval())
}

func TestToProto(t *testing.T) {
	m := &Manifest{
		ID:                   "demo",
		Name:                 "演示",
		Icon:                 "mdi:puzzle",
		Route:                "/demo",
		SortOrder:            10,
		Permission:           "demo:use",
		HealthIntervalSeconds: 20,
	}
	p := m.ToProto()
	require.NotNil(t, p)
	assert.Equal(t, "demo", p.Id)
	assert.Equal(t, "演示", p.Name)
	assert.Equal(t, "/demo", p.Route)
	assert.Equal(t, int32(10), p.SortOrder)
	assert.Equal(t, "demo:use", p.Permission)
	assert.Equal(t, int32(20), p.HealthIntervalSeconds)

	assert.Nil(t, (*Manifest)(nil).ToProto())
}
