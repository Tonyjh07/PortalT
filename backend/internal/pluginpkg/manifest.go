// Package pluginpkg 提供原生插件运行时元数据（manifest.json）的解析与校验。
package pluginpkg

import (
	"encoding/json"
	"fmt"
	"os"

	pluginv1 "portalt/proto/plugin/v1"
)

// DefaultHealthInterval 健康探测默认间隔（秒）。
const DefaultHealthInterval = 30

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
		Id:                   m.ID,
		Name:                 m.Name,
		Icon:                 m.Icon,
		Route:                m.Route,
		SortOrder:            int32(m.SortOrder),
		Permission:           m.Permission,
		HealthIntervalSeconds: int32(m.HealthInterval()),
	}
}
