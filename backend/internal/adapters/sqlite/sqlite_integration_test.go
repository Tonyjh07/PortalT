//go:build integration

package sqlite

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"portalt/internal/domain"
	"portalt/internal/ports"
)

// migrationsDir 解析 backend/migrations/sqlite 目录（测试 cwd 为包目录）。
func migrationsDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	dir := filepath.Join(wd, "..", "..", "..", "migrations", "sqlite")
	info, err := os.Stat(dir)
	require.NoError(t, err, "sqlite 迁移目录不存在: %s", dir)
	require.True(t, info.IsDir())
	return dir
}

// setupTestDB 打开临时文件数据库并应用 SQLite 方言迁移。
func setupTestDB(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "portalt.db")
	db, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, Migrate(db, migrationsDir(t)))

	// 测试结束后关闭连接池，释放 SQLite 文件锁
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db, path
}

func TestSQLite_MigrateAndCrud(t *testing.T) {
	db, path := setupTestDB(t)
	vmRepo := NewVMRepository(db)
	userRepo := NewUserRepository(db)

	// VM 全流程
	vm := &domain.VM{
		ID: "vm-1", Name: "web", Status: domain.VMStatusPoweredOn,
		CPU: 2, MemoryMB: 4096, IPAddress: "10.0.0.5", Host: "esxi-01",
		Metadata: map[string]any{"proto": "vnc"},
	}
	require.NoError(t, vmRepo.Save(vm))
	got, err := vmRepo.FindByID("vm-1")
	require.NoError(t, err)
	assert.Equal(t, vm, got)

	// 用户全流程
	user := &domain.User{ID: "u-1", Username: "admin", Password: "h", Email: "a@b.c", Role: domain.RoleAdmin}
	require.NoError(t, userRepo.Save(user))
	gotUser, err := userRepo.FindByUsername("admin")
	require.NoError(t, err)
	assert.Equal(t, "u-1", gotUser.ID)

	require.NoError(t, vmRepo.Delete("vm-1"))
	_, err = vmRepo.FindByID("vm-1")
	assert.ErrorIs(t, err, ports.ErrNotFound)

	// 迁移幂等（重复执行不报错）
	require.NoError(t, Migrate(db, migrationsDir(t)))

	// 文件确实落盘
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.True(t, info.Size() > 0)
}

func TestSQLite_OpenMemory(t *testing.T) {
	db, err := Open("")
	require.NoError(t, err)
	require.NotNil(t, db)
}

func TestSQLite_Isolation(t *testing.T) {
	// 两个独立数据库文件互不影响
	db1, _ := setupTestDB(t)
	db2, _ := setupTestDB(t)
	r1 := NewVMRepository(db1)
	r2 := NewVMRepository(db2)

	require.NoError(t, r1.Save(&domain.VM{ID: "vm-1", Name: "a"}))
	_, err := r2.FindByID("vm-1")
	assert.ErrorIs(t, err, ports.ErrNotFound)
}
