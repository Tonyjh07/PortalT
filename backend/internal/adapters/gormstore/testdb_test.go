package gormstore

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newTestDB 创建临时文件 SQLite 连接并建表。
// 不依赖任何外部服务，可在 make test-unit 中常驻执行。
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	require.NoError(t, db.AutoMigrate(&vmModel{}, &userModel{}, &pluginModel{}))

	// 测试结束后关闭连接池，释放 SQLite 文件锁
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
