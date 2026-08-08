//go:build integration

package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

func TestPluginRepository_Crud(t *testing.T) {
	db := setupTestDB(t)
	truncateTables(t, db)
	repo := NewPluginRepository(db)

	plugin := &domain.Plugin{
		ID: "p-1", Name: "Home Assistant", Icon: "mdi:home", Route: "/ha",
		Type: domain.PluginTypeAccess, IframeURL: "https://ha.local",
		Permission: "", SortOrder: 2, IsActive: true,
	}
	require.NoError(t, repo.Save(plugin))

	got, err := repo.FindByID("p-1")
	require.NoError(t, err)
	assert.Equal(t, plugin, got)

	// 更新（upsert）
	plugin.Name = "HA Core"
	plugin.SortOrder = 1
	plugin.IsActive = false
	require.NoError(t, repo.Save(plugin))
	got, err = repo.FindByID("p-1")
	require.NoError(t, err)
	assert.Equal(t, "HA Core", got.Name)
	assert.Equal(t, 1, got.SortOrder)
	assert.False(t, got.IsActive)

	// 删除
	require.NoError(t, repo.Delete("p-1"))
	_, err = repo.FindByID("p-1")
	assert.ErrorIs(t, err, ports.ErrNotFound)
	assert.ErrorIs(t, repo.Delete("p-1"), ports.ErrNotFound)
}

func TestPluginRepository_FindActiveSorted(t *testing.T) {
	db := setupTestDB(t)
	truncateTables(t, db)
	repo := NewPluginRepository(db)

	require.NoError(t, repo.Save(&domain.Plugin{ID: "p-2", Name: "b", Route: "/b", SortOrder: 20, IsActive: true}))
	require.NoError(t, repo.Save(&domain.Plugin{ID: "p-1", Name: "a", Route: "/a", SortOrder: 10, IsActive: true}))
	require.NoError(t, repo.Save(&domain.Plugin{ID: "p-3", Name: "c", Route: "/c", SortOrder: 15, IsActive: false}))

	active, err := repo.FindActive()
	require.NoError(t, err)
	require.Len(t, active, 2)
	assert.Equal(t, []string{"p-1", "p-2"}, []string{active[0].ID, active[1].ID})

	all, err := repo.FindAll()
	require.NoError(t, err)
	require.Len(t, all, 3)
	assert.Equal(t, "p-1", all[0].ID)
	assert.Equal(t, "p-3", all[1].ID)
	assert.Equal(t, "p-2", all[2].ID)
}

func TestPluginRepository_NotFoundAndInvalid(t *testing.T) {
	db := setupTestDB(t)
	truncateTables(t, db)
	repo := NewPluginRepository(db)

	_, err := repo.FindByID("missing")
	assert.ErrorIs(t, err, ports.ErrNotFound)

	assert.ErrorIs(t, repo.Save(nil), ports.ErrInvalidArgument)
	assert.ErrorIs(t, repo.Save(&domain.Plugin{}), ports.ErrInvalidArgument)
}
