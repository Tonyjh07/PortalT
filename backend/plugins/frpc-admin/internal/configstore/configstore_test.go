package configstore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)
	return s
}

func TestSaveGetListDelete(t *testing.T) {
	s := newTestStore(t)

	_, ok := s.Get("vm-1")
	assert.False(t, ok)

	c := Connection{
		VMID:   "vm-1",
		VMName: "测试机",
		Host:   "10.0.0.10",
		Port:   22,
		User:   "root",
		Password: "secret",
		ConfigPath: "/etc/frp/frpc.ini",
	}
	require.NoError(t, s.Save(c))

	got, ok := s.Get("vm-1")
	require.True(t, ok)
	assert.Equal(t, "secret", got.Password)
	assert.Equal(t, "/etc/frp/frpc.ini", got.ConfigPath)

	// 更新
	c.Password = "new-secret"
	c.Port = 2222
	require.NoError(t, s.Save(c))
	got, _ = s.Get("vm-1")
	assert.Equal(t, "new-secret", got.Password)
	assert.Equal(t, 2222, got.Port)

	// 多个 + 排序
	require.NoError(t, s.Save(Connection{VMID: "vm-0", Host: "h0", User: "u", Port: 22}))
	list := s.List()
	require.Len(t, list, 2)
	assert.Equal(t, "vm-0", list[0].VMID)
	assert.Equal(t, "vm-1", list[1].VMID)

	// 删除
	require.NoError(t, s.Delete("vm-1"))
	_, ok = s.Get("vm-1")
	assert.False(t, ok)
	err := s.Delete("vm-1")
	assert.Error(t, err, "重复删除应报错")
}

func TestPersistReload(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)
	require.NoError(t, s.Save(Connection{VMID: "vm-9", Host: "10.0.0.9", User: "root", Port: 22, Password: "pw"}))

	// 重新加载（模拟进程重启）
	s2, err := New(dir)
	require.NoError(t, err)
	got, ok := s2.Get("vm-9")
	require.True(t, ok)
	assert.Equal(t, "pw", got.Password)
	assert.Equal(t, 22, got.Port)
}

func TestFilePermissionAndAtomic(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	require.NoError(t, err)
	require.NoError(t, s.Save(Connection{VMID: "vm-1", Host: "h", User: "u", Port: 22}))

	// 文件存在且无 .tmp 残留
	assert.FileExists(t, filepath.Join(dir, "connections.json"))
	_, err = os.Stat(filepath.Join(dir, "connections.json.tmp"))
	assert.True(t, os.IsNotExist(err), "不应残留临时文件")
}

func TestValidate(t *testing.T) {
	s := newTestStore(t)
	assert.Error(t, s.Save(Connection{Host: "h", User: "u", Port: 22}), "缺 vm_id")
	assert.Error(t, s.Save(Connection{VMID: "v", User: "u", Port: 22}), "缺 host")
	assert.Error(t, s.Save(Connection{VMID: "v", Host: "h", Port: 22}), "缺 user")
	assert.Error(t, s.Save(Connection{VMID: "v", Host: "h", User: "u", Port: 0}), "缺 port")
	assert.Error(t, s.Save(Connection{VMID: "v", Host: "h", User: "u", Port: 99999}), "端口越界")
}

func TestSanitize(t *testing.T) {
	c := Connection{VMID: "v", Password: "p", SudoPassword: "sp"}
	san := Sanitize(c)
	assert.Equal(t, "", san.Password)
	assert.Equal(t, "", san.SudoPassword)
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "connections.json"), []byte("{bad"), 0o600))
	_, err := New(dir)
	assert.Error(t, err)
}
