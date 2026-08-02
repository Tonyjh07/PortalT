package services

import (
	"context"
	"io/fs"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/domain"
	"portalt/internal/plugins"
	"portalt/internal/ports"
)

type pluginStubRepo struct {
	byID map[string]*domain.Plugin
}

func (r *pluginStubRepo) Save(p *domain.Plugin) error {
	if p == nil || p.ID == "" {
		return ports.ErrInvalidArgument
	}
	r.byID[p.ID] = p
	return nil
}

func (r *pluginStubRepo) FindByID(id string) (*domain.Plugin, error) {
	if p, ok := r.byID[id]; ok {
		return p, nil
	}
	return nil, ports.ErrNotFound
}

func (r *pluginStubRepo) Delete(id string) error {
	if _, ok := r.byID[id]; !ok {
		return ports.ErrNotFound
	}
	delete(r.byID, id)
	return nil
}

func (r *pluginStubRepo) FindActive() ([]*domain.Plugin, error)  { return nil, nil }
func (r *pluginStubRepo) FindAll() ([]*domain.Plugin, error)     { return nil, nil }

type nativeStub struct{ info domain.Plugin }

func (s nativeStub) Info() domain.Plugin { return s.info }
func (s nativeStub) Mount(_ *gin.RouterGroup, _ plugins.Deps) {
	// 测试中不需要挂载
}
func (s nativeStub) StaticFS() fs.FS { return nil }

func TestSyncNativePlugins_Insert(t *testing.T) {
	reg := plugins.NewRegistry()
	require.NoError(t, reg.Register(nativeStub{info: domain.Plugin{
		ID: "esxi-admin", Name: "ESXi 管理", Icon: "mdi:server", Route: "/esxi-admin", SortOrder: 90,
	}}))

	repo := &pluginStubRepo{byID: map[string]*domain.Plugin{}}
	n, err := SyncNativePlugins(context.Background(), repo, reg)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	got := repo.byID["esxi-admin"]
	require.NotNil(t, got)
	assert.Equal(t, domain.PluginTypeNative, got.Type)
	assert.Equal(t, 90, got.SortOrder)
}

func TestSyncNativePlugins_KeepAdminSettings(t *testing.T) {
	reg := plugins.NewRegistry()
	require.NoError(t, reg.Register(nativeStub{info: domain.Plugin{
		ID: "esxi-admin", Name: "ESXi 管理", Route: "/esxi-admin",
	}}))

	// 管理员已停用并配了权限 → 同步后必须保留
	repo := &pluginStubRepo{byID: map[string]*domain.Plugin{
		"esxi-admin": {ID: "esxi-admin", Name: "旧名", Type: domain.PluginTypeNative,
			Permission: "plugin:view", IsActive: false, SortOrder: 5},
	}}
	_, err := SyncNativePlugins(context.Background(), repo, reg)
	require.NoError(t, err)

	got := repo.byID["esxi-admin"]
	assert.False(t, got.IsActive, "启用状态应保留管理员设置")
	assert.Equal(t, "plugin:view", got.Permission, "权限应保留管理员设置")
	assert.Equal(t, "ESXi 管理", got.Name, "名称应跟随代码更新")
	assert.Equal(t, 5, got.SortOrder, "非零 SortOrder 保留")
}

func TestSyncNativePlugins_DefaultSortOrder(t *testing.T) {
	reg := plugins.NewRegistry()
	require.NoError(t, reg.Register(nativeStub{info: domain.Plugin{
		ID: "x", Name: "X", Route: "/x",
	}}))
	repo := &pluginStubRepo{byID: map[string]*domain.Plugin{
		"x": {ID: "x", Type: domain.PluginTypeNative, SortOrder: 0},
	}}
	_, err := SyncNativePlugins(context.Background(), repo, reg)
	require.NoError(t, err)
	assert.Equal(t, 100, repo.byID["x"].SortOrder)
}
