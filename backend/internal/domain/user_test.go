package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// testUser 构造指定角色的用户
func testUser(role Role) *User {
	return &User{
		ID:       "u-1",
		Username: "tester",
		Password: "hash",
		Email:    "tester@lab.local",
		Role:     role,
	}
}

func TestUser_HasPermission(t *testing.T) {
	tests := []struct {
		name string
		user *User
		perm string
		want bool
	}{
		{"管理员可启动VM", testUser(RoleAdmin), PERM_VM_START, true},
		{"管理员可管理用户", testUser(RoleAdmin), PERM_USER_MANAGE, true},
		{"普通用户可启动VM", testUser(RoleUser), PERM_VM_START, true},
		{"普通用户不可管理用户", testUser(RoleUser), PERM_USER_MANAGE, false},
		{"访客可查看VM", testUser(RoleViewer), PERM_VM_VIEW, true},
		{"访客不可启动VM", testUser(RoleViewer), PERM_VM_START, false},
		{"访客不可管理插件", testUser(RoleViewer), PERM_PLUGIN_MANAGE, false},
		{"未知角色无权限", testUser("ghost"), PERM_VM_VIEW, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.user.HasPermission(tt.perm))
		})
	}
}

func TestUser_HasPermission_Nil(t *testing.T) {
	// nil用户不应panic，应返回false
	var u *User
	assert.False(t, u.HasPermission(PERM_VM_VIEW))
}

func TestUser_HasPermission_InvalidPerm(t *testing.T) {
	// 未定义的权限字符串不应命中
	assert.False(t, testUser(RoleAdmin).HasPermission("no:such:perm"))
}

func TestUser_IsAdmin(t *testing.T) {
	assert.True(t, testUser(RoleAdmin).IsAdmin())
	assert.False(t, testUser(RoleUser).IsAdmin())
	assert.False(t, testUser(RoleViewer).IsAdmin())
	var u *User
	assert.False(t, u.IsAdmin())
}

func TestUser_HasPermission_CoversAllDefinedPerms(t *testing.T) {
	// 保护性约束：所有定义的权限常量都必须在管理员角色中可访问
	allPerms := []string{
		PERM_VIEW_ALL, PERM_VM_VIEW, PERM_VM_START, PERM_VM_STOP,
		PERM_VM_RESTART, PERM_VM_MANAGE, PERM_PLUGIN_VIEW,
		PERM_PLUGIN_MANAGE, PERM_USER_MANAGE,
	}
	admin := testUser(RoleAdmin)
	for _, perm := range allPerms {
		assert.Truef(t, admin.HasPermission(perm), "管理员应拥有权限 %s", perm)
	}
}

func TestUser_JSONFields(t *testing.T) {
	// 密码字段必须被隐藏，不进入JSON输出
	u := testUser(RoleUser)
	json := `{"id":"u-1","username":"tester","email":"tester@lab.local","role":"user"}`
	assert.Equal(t, json, string(mustMarshal(t, u)))
	assert.NotContains(t, string(mustMarshal(t, u)), "hash")
}
