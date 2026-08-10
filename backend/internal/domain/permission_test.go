package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPermission_ConstantsAreStrings(t *testing.T) {
	// 权限常量应遵循 "资源:动作" 约定，且不为空
	perms := []string{
		PERM_VIEW_ALL, PERM_VM_VIEW, PERM_VM_START, PERM_VM_STOP,
		PERM_VM_RESTART, PERM_VM_MANAGE, PERM_PLUGIN_VIEW,
		PERM_PLUGIN_MANAGE, PERM_USER_MANAGE,
		PERM_ESXI_ADMIN_USE, PERM_FRPC_ADMIN_MANAGE,
	}
	for _, perm := range perms {
		assert.NotEmpty(t, perm)
		assert.NotEqual(t, "unknown", perm)
	}
}

func TestRole_Constants(t *testing.T) {
	assert.Equal(t, Role("admin"), RoleAdmin)
	assert.Equal(t, Role("user"), RoleUser)
	assert.Equal(t, Role("viewer"), RoleViewer)
}
