// Package postgres 提供基于 PostgreSQL + GORM 的适配器实现。
//
// 与 memory 适配器共享同一组 ports 接口，验证存储可替换性。
// 仓储使用独立的数据库模型（vmModel/userModel），保持 domain 层纯净，
// 不引入框架标签与第三方类型依赖。
package postgres

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open 使用 DSN 建立 PostgreSQL 连接。
// DSN 示例：postgres://user:pass@localhost:5432/dbname?sslmode=disable
func Open(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	return db, nil
}

// Migrate 按文件名顺序执行目录下的 *.up.sql 迁移脚本。
// 脚本通过 GORM 连接执行，适用于测试与启动时初始化。
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
