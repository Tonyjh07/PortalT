// Package sqlite 提供基于 SQLite 的适配器实现（部署/调试便捷模式）。
//
// 使用纯 Go 驱动（glebarez/sqlite，底层 modernc.org/sqlite），
// 无需 CGO 与外部服务，单文件数据库即可运行。
// 仓储逻辑复用共享的 gormstore 包，与 PostgreSQL 实现行为一致。
package sqlite

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open 打开 SQLite 数据库文件。
// path 为空或 ":memory:" 时使用内存数据库（仅测试用）。
func Open(path string) (*gorm.DB, error) {
	if path == "" {
		path = ":memory:"
	}
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}
	return db, nil
}

// Migrate 按文件名顺序执行目录下的 *.up.sql SQLite 方言迁移脚本。
// 已应用的迁移记录在 schema_migrations 表，重复执行自动跳过（幂等）。
//
// 兼容旧库：SQLite 不支持 ADD COLUMN IF NOT EXISTS，早期版本（无版本表
// 时期）重放 ALTER 会报 "duplicate column name"，此类错误视为已应用。
func Migrate(db *gorm.DB, migrationsDir string) error {
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	if err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version    TEXT PRIMARY KEY,
		applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`).Error; err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	applied, err := appliedVersions(db)
	if err != nil {
		return err
	}

	for _, name := range files {
		version := strings.TrimSuffix(name, ".up.sql")
		if applied[version] {
			continue
		}
		b, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if err := db.Exec(string(b)).Error; err != nil {
			if isReplayableErr(err) {
				if err := recordVersion(db, version); err != nil {
					return fmt.Errorf("record migration %s: %w", name, err)
				}
				continue
			}
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if err := recordVersion(db, version); err != nil {
			return fmt.Errorf("record migration %s: %w", name, err)
		}
	}
	return nil
}

func appliedVersions(db *gorm.DB) (map[string]bool, error) {
	out := make(map[string]bool)
	rows, err := db.Raw(`SELECT version FROM schema_migrations`).Rows()
	if err != nil {
		return nil, fmt.Errorf("query schema_migrations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out[v] = true
	}
	return out, rows.Err()
}

func recordVersion(db *gorm.DB, version string) error {
	return db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, version).Error
}

// isReplayableErr 判断是否为旧库重放非幂等 DDL 的已知错误
// （SQLite 无 ADD COLUMN IF NOT EXISTS，重复加列报 duplicate column name）。
func isReplayableErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate column name")
}
