// Package configstore 持久化目标主机的 SSH 连接配置与 frpc 管理参数。
//
// 存储：插件数据目录（data/connections.json），0600 权限，原子写（临时文件+rename）。
// 凭据（密码/sudo 密码）落盘保存；返回给前端时需脱敏（密码只写不回）。
// 单连接模型：插件管理一台目标主机，不依赖 PortalT 的 VM 概念（与主程序解耦）。
package configstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ErrNotFound 目标连接配置不存在。
var ErrNotFound = errors.New("连接配置不存在")

// Connection 目标主机的 SSH 连接与 frpc 管理配置。
type Connection struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	User         string `json:"user"`
	Password     string `json:"password"`
	SudoEnabled  bool   `json:"sudo_enabled"` // 是否经 sudo 写文件/重启
	SudoPassword string `json:"sudo_password"`
	// ConfigPath frpc 配置文件路径（缺省自动探测）。
	ConfigPath string `json:"config_path"`
	// Format 配置格式：auto/ini/toml（auto 时按探测或文件后缀）。
	Format string `json:"format"`
	// RestartCmd 重启 frpc 命令（缺省 systemctl restart frpc）。
	RestartCmd string `json:"restart_cmd"`
	// sudo 决策（api 层 sudoPasswordFor）：
	// SudoEnabled 关闭 → 不包 sudo（root 登录或 sudo 无需密码）；
	// 开启 → SudoPassword 非空用之，否则回退 Password。
}

// Store 单连接配置存储。
type Store struct {
	mu   sync.Mutex
	path string
	conn *Connection // nil = 未配置
}

// New 加载/初始化存储。dataDir 为插件数据目录。
func New(dataDir string) (*Store, error) {
	s := &Store{path: filepath.Join(dataDir, "connections.json")}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("读取连接配置失败: %w", err)
	}
	if len(trimBytes(b)) == 0 {
		return nil
	}
	var c Connection
	if err := json.Unmarshal(b, &c); err == nil && c.Host != "" {
		// 新单连接格式（含 host 字段说明非 legacy map）
		s.conn = &c
		return nil
	}
	// 兼容旧版多连接格式（map[vm_id]Connection）：取任意一条
	if migrated := migrateLegacy(b); migrated != nil {
		s.conn = migrated
		return nil
	}
	// 兜底：尝试单对象解析；空对象（删除后落盘 {}）视为未配置
	if err := json.Unmarshal(b, &c); err != nil {
		return fmt.Errorf("解析连接配置失败: %w", err)
	}
	if c.Host != "" {
		s.conn = &c
	}
	return nil
}

// Get 读取连接配置。
func (s *Store) Get() (Connection, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return Connection{}, false
	}
	return *s.conn, true
}

// Save 保存（或更新）连接配置。
func (s *Store) Save(c Connection) error {
	if err := validate(c); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conn = &c
	return s.persistLocked()
}

// Delete 删除连接配置。
func (s *Store) Delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.conn == nil {
		return ErrNotFound
	}
	s.conn = nil
	return s.persistLocked()
}

// persistLocked 原子写盘（调用方需持有锁）。
func (s *Store) persistLocked() error {
	var b []byte
	var err error
	if s.conn == nil {
		b, err = json.MarshalIndent(map[string]any{}, "", "  ")
	} else {
		b, err = json.MarshalIndent(s.conn, "", "  ")
	}
	if err != nil {
		return fmt.Errorf("序列化连接配置失败: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("写临时配置文件失败: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("更新配置文件失败: %w", err)
	}
	return nil
}

// validate 基础校验。
func validate(c Connection) error {
	if c.Host == "" {
		return errors.New("SSH 主机不能为空")
	}
	if c.User == "" {
		return errors.New("SSH 用户名不能为空")
	}
	if c.Port == 0 {
		return errors.New("SSH 端口不能为空")
	}
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("SSH 端口必须在 1-65535")
	}
	return nil
}

// Sanitize 返回脱敏副本（密码/sudo 密码置空，避免回传前端）。
func Sanitize(c Connection) Connection {
	c.Password = ""
	c.SudoPassword = ""
	return c
}
