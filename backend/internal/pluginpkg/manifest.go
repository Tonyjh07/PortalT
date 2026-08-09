// Package pluginpkg 提供原生插件运行时元数据（manifest.json）的解析与校验。
package pluginpkg

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"

	pluginv1 "portalt/proto/plugin/v1"
)

// DefaultHealthInterval 健康探测默认间隔（秒）。
const DefaultHealthInterval = 30

// pluginIDPattern 插件 ID 字符集白名单：字母数字开头，仅含字母数字与 . _ -，
// 与插件管理 API（api/v1/plugin.go）的 ID 约束一致。ID 用于 URL 路径、
// plugins 表主键与 Caddy 规则文件名，防止特殊字符（含 ../）造成路径穿越或
// 路由冲突。
var pluginIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// Manifest 原生插件 manifest.json 结构。
// 字段与 proto 的 Manifest 一一对应，供启动握手时做一致性校验。
type Manifest struct {
	// ID 插件 ID（= plugins 表 id = 插件目录名）
	ID string `json:"id"`
	// Name 显示名称
	Name string `json:"name"`
	// Icon 图标标识（如 "mdi:puzzle"）
	Icon string `json:"icon"`
	// Route 前端路由路径（约定 "/"+ID）
	Route string `json:"route"`
	// SortOrder 排序权重，值越小越靠前
	SortOrder int `json:"sort_order"`
	// Permission 访问所需最小权限（空 = 无需额外权限）
	Permission string `json:"permission"`
	// HealthIntervalSeconds 健康探测间隔（秒），<=0 时用默认值
	HealthIntervalSeconds int `json:"health_interval_seconds"`
	// Version 插件版本号（如 "1.2.0"），管理界面展示与兼容性判断
	Version string `json:"version"`
	// Description 插件一句话描述，管理界面展示
	Description string `json:"description"`
}

// Load 从路径读取并解析 manifest.json；文件缺失或内容非法返回错误。
func Load(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 manifest %q: %w", path, err)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("解析 manifest %q: %w", path, err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Validate 校验 manifest 必要字段。
func (m *Manifest) Validate() error {
	if m == nil {
		return fmt.Errorf("manifest 不能为空")
	}
	if m.ID == "" {
		return fmt.Errorf("manifest: 插件 ID 不能为空")
	}
	if !pluginIDPattern.MatchString(m.ID) {
		return fmt.Errorf("manifest: 插件 ID %q 非法（仅允许字母数字及 . _ -，且须以字母或数字开头）", m.ID)
	}
	if m.Name == "" {
		return fmt.Errorf("manifest %q: 名称不能为空", m.ID)
	}
	if m.Route == "" {
		return fmt.Errorf("manifest %q: 路由不能为空", m.ID)
	}
	if m.Route[0] != '/' {
		return fmt.Errorf("manifest %q: 路由须以 / 开头（当前 %q）", m.ID, m.Route)
	}
	return nil
}

// HealthInterval 返回健康探测间隔；未设置或非法时返回默认值。
func (m *Manifest) HealthInterval() int {
	if m == nil || m.HealthIntervalSeconds <= 0 {
		return DefaultHealthInterval
	}
	return m.HealthIntervalSeconds
}

// ToProto 转换为握手用的 proto.Manifest。
func (m *Manifest) ToProto() *pluginv1.Manifest {
	if m == nil {
		return nil
	}
	return &pluginv1.Manifest{
		Id:                    m.ID,
		Name:                  m.Name,
		Icon:                  m.Icon,
		Route:                 m.Route,
		SortOrder:             int32(m.SortOrder),
		Permission:            m.Permission,
		HealthIntervalSeconds: int32(m.HealthInterval()),
		Version:               m.Version,
		Description:           m.Description,
	}
}
