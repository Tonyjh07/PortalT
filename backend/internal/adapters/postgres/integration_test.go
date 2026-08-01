//go:build integration

// Package postgres 集成测试。
//
// 依赖真实 PostgreSQL（docker compose up -d postgres），
// 通过环境变量 TEST_DATABASE_URL 指定连接，默认使用 compose 默认凭据。
package postgres

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// defaultTestDSN 与 docker-compose.yml 默认凭据一致
const defaultTestDSN = "postgres://portalt:securepassword@localhost:5432/portalt?sslmode=disable"

// setupTestDB 连接数据库并应用迁移，返回可用的 GORM 连接。
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = defaultTestDSN
	}
	db, err := Open(dsn)
	require.NoError(t, err, "连接 PostgreSQL 失败（需先执行 docker compose up -d postgres）")

	migrationsDir, err := migrationsDir()
	require.NoError(t, err)
	require.NoError(t, Migrate(db, migrationsDir))
	return db
}

// migrationsDir 解析 backend/migrations 目录（测试 cwd 为包目录）。
func migrationsDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(wd, "..", "..", "..", "migrations")
	info, err := os.Stat(dir)
	if err != nil {
		return "", fmt.Errorf("stat migrations dir %s: %w", dir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("migrations dir %s is not a directory", dir)
	}
	return dir, nil
}

// truncateTables 清空业务表，保证测试间隔离。
func truncateTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.Exec("TRUNCATE TABLE users, vms, plugins, permissions CASCADE").Error)
}
