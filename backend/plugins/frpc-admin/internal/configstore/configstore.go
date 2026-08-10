// Package configstore 持久化各 VM 的 SSH 连接配置与 frpc 管理参数。
//
// 存储：插件数据目录（data/connections.json），0600 权限，原子写（临时文件+rename）。
// 凭据（密码/sudo 密码）落盘保存；返回给前端时需脱敏（密码只写不回）。
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

// Connection 某台 VM 的 SSH 连接与 frpc 管理配置。
type Connection struct {
	VMID        string `json:"vm_id"`
	VMName      string `json:"vm_name"` // 前端展示用（非权威，可刷新）
	Host        string `json:"host"`
	Port        int    `json:"port"`
	User        string `json:"user"`
	Password    string `json:"password"`
	SudoEnabled bool   `json:"sudo_enabled"` // 是否经 sudo 写文件/重启
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

// Store 连接配置存储。
type Store struct {
	mu   sync.Mutex
	path string
	data map[string]Connection // key = VMID
}

// New 加载/初始化存储。dataDir 为插件数据目录。
func New(dataDir string) (*Store, error) {
	s := &Store{
		path: filepath.Join(dataDir, "connections.json"),
		data: map[string]Connection{},
	}
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
	if len(bytesTrim(b)) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return fmt.Errorf("解析连接配置失败: %w", err)
	}
	if s.data == nil {
		s.data = map[string]Connection{}
	}
	return nil
}

// Save 保存（或更新）某 VM 的连接配置。
func (s *Store) Save(c Connection) error {
	if err := validate(c); err != nil {
		return err
	}
	s.mu.Lock()
	s.data[c.VMID] = c
	err := s.persistLocked()
	s.mu.Unlock()
	return err
}

// Get 读取某 VM 的连接配置。
func (s *Store) Get(vmID string) (Connection, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.data[vmID]
	return c, ok
}

// List 返回全部连接配置（按 VMID 排序，便于前端稳定展示）。
func (s *Store) List() []Connection {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Connection, 0, len(s.data))
	ids := make([]string, 0, len(s.data))
	for id := range s.data {
		ids = append(ids, id)
	}
	sortStrings(ids)
	for _, id := range ids {
		out = append(out, s.data[id])
	}
	return out
}

// Delete 删除某 VM 的连接配置。
func (s *Store) Delete(vmID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data[vmID]; !ok {
		return ErrNotFound
	}
	delete(s.data, vmID)
	return s.persistLocked()
}

// persistLocked 原子写盘（调用方需持有锁）。
func (s *Store) persistLocked() error {
	b, err := json.MarshalIndent(s.data, "", "  ")
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
	if c.VMID == "" {
		return errors.New("vm_id 不能为空")
	}
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
