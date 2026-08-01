package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// testPlugin 构造插件
func testPlugin(active bool, perm string) *Plugin {
	return &Plugin{
		ID:         "p-1",
		Name:       "Home Assistant",
		Icon:       "mdi:home",
		Route:      "/ha",
		IframeURL:  "https://ha.local",
		Permission: perm,
		SortOrder:  1,
		IsActive:   active,
	}
}

func TestPlugin_CanAccess(t *testing.T) {
	tests := []struct {
		name   string
		plugin *Plugin
		user   *User
		want   bool
	}{
		{"启用且无权限要求，任意用户可访问", testPlugin(true, ""), testUser(RoleViewer), true},
		{"启用且权限满足可访问", testPlugin(true, PERM_VM_START), testUser(RoleUser), true},
		{"启用但权限不足不可访问", testPlugin(true, PERM_USER_MANAGE), testUser(RoleViewer), false},
		{"未启用不可访问", testPlugin(false, ""), testUser(RoleAdmin), false},
		{"未启用且权限满足也不可访问", testPlugin(false, PERM_VM_START), testUser(RoleAdmin), false},
		{"nil用户不可访问", testPlugin(true, ""), nil, false},
		{"nil插件不可访问", nil, testUser(RoleAdmin), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.plugin.CanAccess(tt.user))
		})
	}
}

func TestPlugin_CanAccess_AdminPermission(t *testing.T) {
	// 管理员应可访问任何启用且权限已定义的插件
	admin := testUser(RoleAdmin)
	for _, perm := range []string{PERM_VM_VIEW, PERM_PLUGIN_MANAGE, PERM_USER_MANAGE} {
		assert.True(t, testPlugin(true, perm).CanAccess(admin))
	}
}

func TestPlugin_IsEnabled(t *testing.T) {
	assert.True(t, testPlugin(true, "").IsEnabled())
	assert.False(t, testPlugin(false, "").IsEnabled())
	var p *Plugin
	assert.False(t, p.IsEnabled())
}

func TestPlugin_JSONFields(t *testing.T) {
	p := testPlugin(true, PERM_PLUGIN_VIEW)
	json := `{"id":"p-1","name":"Home Assistant","icon":"mdi:home","route":"/ha","iframe_url":"https://ha.local","permission":"plugin:view","sort_order":1,"is_active":true}`
	assert.Equal(t, json, string(mustMarshal(t, p)))
}
