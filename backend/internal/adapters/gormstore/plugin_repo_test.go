package gormstore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

func newPluginRepo(t *testing.T) *PluginRepository {
	t.Helper()
	return NewPluginRepository(newTestDB(t))
}

func TestPluginRepo_SaveAndFind(t *testing.T) {
	r := newPluginRepo(t)
	p := &domain.Plugin{ID: "p-1", Name: "Home Assistant", Icon: "mdi:home", Route: "/ha",
		IframeURL: "https://ha.local", Permission: "", SortOrder: 1, IsActive: true}

	require.NoError(t, r.Save(p))

	got, err := r.FindByID("p-1")
	require.NoError(t, err)
	assert.Equal(t, "p-1", got.ID)
	assert.Equal(t, "/ha", got.Route)
	assert.Equal(t, "https://ha.local", got.IframeURL)
	assert.Equal(t, 1, got.SortOrder)
	assert.True(t, got.IsActive)
}

func TestPluginRepo_Save_Upsert(t *testing.T) {
	r := newPluginRepo(t)
	require.NoError(t, r.Save(&domain.Plugin{ID: "p-1", Name: "old", Route: "/ha"}))
	require.NoError(t, r.Save(&domain.Plugin{ID: "p-1", Name: "new", Route: "/ha", SortOrder: 5, IsActive: false}))

	got, err := r.FindByID("p-1")
	require.NoError(t, err)
	assert.Equal(t, "new", got.Name)
	assert.Equal(t, 5, got.SortOrder)
	assert.False(t, got.IsActive)
}

func TestPluginRepo_Save_Invalid(t *testing.T) {
	r := newPluginRepo(t)
	assert.ErrorIs(t, r.Save(nil), ports.ErrInvalidArgument)
	assert.ErrorIs(t, r.Save(&domain.Plugin{Name: "no-id"}), ports.ErrInvalidArgument)
}

func TestPluginRepo_FindByID_NotFound(t *testing.T) {
	r := newPluginRepo(t)
	_, err := r.FindByID("ghost")
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

func TestPluginRepo_FindActive_Sorted(t *testing.T) {
	r := newPluginRepo(t)
	require.NoError(t, r.Save(&domain.Plugin{ID: "p-3", Name: "z", Route: "/z", SortOrder: 30, IsActive: true}))
	require.NoError(t, r.Save(&domain.Plugin{ID: "p-1", Name: "a", Route: "/a", SortOrder: 10, IsActive: true}))
	require.NoError(t, r.Save(&domain.Plugin{ID: "p-2", Name: "b", Route: "/b", SortOrder: 20, IsActive: false}))

	active, err := r.FindActive()
	require.NoError(t, err)
	require.Len(t, active, 2)
	assert.Equal(t, "p-1", active[0].ID)
	assert.Equal(t, "p-3", active[1].ID)

	all, err := r.FindAll()
	require.NoError(t, err)
	require.Len(t, all, 3)
}

func TestPluginRepo_FindAll_Empty(t *testing.T) {
	r := newPluginRepo(t)
	all, err := r.FindAll()
	require.NoError(t, err)
	assert.Empty(t, all)
}

func TestPluginRepo_Delete(t *testing.T) {
	r := newPluginRepo(t)
	require.NoError(t, r.Save(&domain.Plugin{ID: "p-1", Name: "x", Route: "/x"}))

	require.NoError(t, r.Delete("p-1"))
	_, err := r.FindByID("p-1")
	assert.ErrorIs(t, err, ports.ErrNotFound)

	assert.ErrorIs(t, r.Delete("p-1"), ports.ErrNotFound)
}
