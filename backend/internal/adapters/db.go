// Package adapters 提供可替换适配器的装配入口（数据库工厂）。
package adapters

import (
	"context"
	"fmt"
	"os"
	"strings"

	"gorm.io/gorm"

	"portalt/internal/adapters/postgres"
	"portalt/internal/adapters/sqlite"
)

// DBDriver 数据库驱动类型。
type DBDriver string

const (
	// DBPostgres PostgreSQL 驱动
	DBPostgres DBDriver = "postgres"
	// DBSQLite SQLite 驱动（单文件，无需外部服务）
	DBSQLite DBDriver = "sqlite"
)

// OpenDB 根据驱动与连接串打开数据库。
// postgres: dsn 为连接串（postgres://user:pass@host:5432/db?sslmode=disable）
// sqlite:   dsn 为数据库文件路径，空串使用内存库（仅测试）
func OpenDB(driver, dsn string) (*gorm.DB, error) {
	switch DBDriver(strings.ToLower(driver)) {
	case DBPostgres:
		return postgres.Open(dsn)
	case DBSQLite:
		return sqlite.Open(dsn)
	default:
		return nil, fmt.Errorf("unsupported db driver %q (supported: postgres, sqlite)", driver)
	}
}

// OpenDBFromEnv 从环境变量打开数据库并应用迁移。
//
//	DB_DRIVER  postgres | sqlite（默认 sqlite，便于调试部署）
//	DB_DSN     postgres连接串 | sqlite文件路径
//	DB_MIGRATIONS_DIR  迁移脚本目录（默认 backend/migrations）
//
// SQLite 的迁移脚本位于 <DB_MIGRATIONS_DIR>/sqlite 子目录。
func OpenDBFromEnv(ctx context.Context) (*gorm.DB, error) {
	driver := strings.ToLower(os.Getenv("DB_DRIVER"))
	if driver == "" {
		driver = string(DBSQLite)
	}
	dsn := os.Getenv("DB_DSN")

	db, err := OpenDB(driver, dsn)
	if err != nil {
		return nil, err
	}

	migrateDir := os.Getenv("DB_MIGRATIONS_DIR")
	if migrateDir == "" {
		migrateDir = "migrations"
	}

	switch DBDriver(driver) {
	case DBPostgres:
		err = postgres.Migrate(db, migrateDir)
	case DBSQLite:
		err = sqlite.Migrate(db, migrateDir+string(os.PathSeparator)+"sqlite")
	}
	if err != nil {
		return nil, fmt.Errorf("apply migrations: %w", err)
	}
	return db, nil
}
