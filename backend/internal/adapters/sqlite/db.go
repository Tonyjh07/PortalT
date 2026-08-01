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

	for _, name := range files {
		b, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if err := db.Exec(string(b)).Error; err != nil {
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
	}
	return nil
}
