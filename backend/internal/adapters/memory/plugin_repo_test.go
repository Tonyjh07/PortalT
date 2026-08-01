package memory

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

func TestPluginRepository_SaveAndFind(t *testing.T) {
	r := NewPluginRepository()
	p := &domain.Plugin{ID: "p-1", Name: "Home Assistant", Route: "/ha", SortOrder: 1, IsActive: true}

	require.NoError(t, r.Save(p))

	got, err := r.FindByID("p-1")
	require.NoError(t, err)
	assert.Equal(t, "p-1", got.ID)
	assert.Equal(t, "/ha", got.Route)
}

func TestPluginRepository_Save_Upsert(t *testing.T) {
	r := NewPluginRepository()
	require.NoError(t, r.Save(&domain.Plugin{ID: "p-1", Name: "old", Route: "/ha"}))
	require.NoError(t, r.Save(&domain.Plugin{ID: "p-1", Name: "new", Route: "/ha", SortOrder: 5}))

	got, err := r.FindByID("p-1")
	require.NoError(t, err)
	assert.Equal(t, "new", got.Name)
	assert.Equal(t, 5, got.SortOrder)
}

func TestPluginRepository_Save_Invalid(t *testing.T) {
	r := NewPluginRepository()
	assert.ErrorIs(t, r.Save(nil), ports.ErrInvalidArgument)
	assert.ErrorIs(t, r.Save(&domain.Plugin{Name: "no-id"}), ports.ErrInvalidArgument)
}

func TestPluginRepository_FindByID_NotFound(t *testing.T) {
	r := NewPluginRepository()
	_, err := r.FindByID("ghost")
	assert.ErrorIs(t, err, ports.ErrNotFound)
}

func TestPluginRepository_FindActive_Sorted(t *testing.T) {
	r := NewPluginRepository()
	require.NoError(t, r.Save(&domain.Plugin{ID: "p-3", Name: "z", SortOrder: 30, IsActive: true}))
	require.NoError(t, r.Save(&domain.Plugin{ID: "p-1", Name: "a", SortOrder: 10, IsActive: true}))
	require.NoError(t, r.Save(&domain.Plugin{ID: "p-2", Name: "b", SortOrder: 20, IsActive: false}))

	active, err := r.FindActive()
	require.NoError(t, err)
	require.Len(t, active, 2)
	assert.Equal(t, "p-1", active[0].ID)
	assert.Equal(t, "p-3", active[1].ID)

	all, err := r.FindAll()
	require.NoError(t, err)
	require.Len(t, all, 3)
}

func TestPluginRepository_FindAll_Empty(t *testing.T) {
	r := NewPluginRepository()
	all, err := r.FindAll()
	require.NoError(t, err)
	assert.Empty(t, all)
}

func TestPluginRepository_Delete(t *testing.T) {
	r := NewPluginRepository()
	require.NoError(t, r.Save(&domain.Plugin{ID: "p-1", Name: "x", Route: "/x"}))

	require.NoError(t, r.Delete("p-1"))
	_, err := r.FindByID("p-1")
	assert.ErrorIs(t, err, ports.ErrNotFound)

	assert.ErrorIs(t, r.Delete("p-1"), ports.ErrNotFound)
}
