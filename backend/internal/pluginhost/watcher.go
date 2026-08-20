package pluginhost

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// watchDebounce 目录变动事件去抖窗口：合并同一插件目录连续事件。
const watchDebounce = 300 * time.Millisecond

// watch 递归监听 PLUGINS_DIR（根目录 + 每个插件子目录），驱动热加载：
//   - 新增目录 → inspect + upsert（新插件默认禁用，仅登记不 spawn）
//   - manifest / 二进制替换 → 重启插件（升级；先停再启）
//   - 目录删除 → 停止进程，DB 标记 missing
//
// fsnotify 不递归，故需对每个插件子目录单独 Add，并在目录创建/删除时动态
// 增删监听；否则 <id>/manifest.json 与 <id>/plugin 的就地替换不会产生根目录
// 事件，热升级（update.sh 就地覆盖产物）将永远不被感知。
func (m *Manager) watch(ctx context.Context) {
	if m.Disabled() {
		return
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		m.logf("fsnotify 初始化失败: %v", err)
		return
	}
	defer func() { _ = watcher.Close() }()

	if err := watcher.Add(m.dir); err != nil {
		m.logf("监听插件目录失败 %s: %v", m.dir, err)
		return
	}
	// 递归监听：初始 Add 各插件子目录；记录已监听集合，供动态增删
	watched := map[string]bool{m.dir: true}
	addSubdirs := func() {
		entries, err := os.ReadDir(m.dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			sub := filepath.Join(m.dir, e.Name())
			if watched[sub] {
				continue
			}
			if err := watcher.Add(sub); err != nil {
				// 监听失败：该插件子目录的就地升级将不被感知，须明确告警
				m.logf("监听插件子目录失败 %s: %v", sub, err)
			} else {
				watched[sub] = true
			}
		}
	}
	addSubdirs()
	m.logf("插件目录热加载已启用: %s", m.dir)

	// 事件合并窗口：收集片段，统一处理，避免同一操作触发多次 spawn
	pending := map[string]bool{}
	var timer *time.Timer
	var timerC <-chan time.Time
	flush := func() {
		if len(pending) == 0 {
			return
		}
		ids := make([]string, 0, len(pending))
		for id := range pending {
			ids = append(ids, id)
		}
		pending = map[string]bool{}
		m.applyEvents(ctx, ids)
	}
	queue := func(id string) {
		pending[id] = true
		if timer == nil {
			timer = time.NewTimer(watchDebounce)
			timerC = timer.C
		} else {
			timer.Reset(watchDebounce)
		}
	}

	for {
		select {
		case <-ctx.Done():
			flush()
			return
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			m.logf("fsnotify 错误: %v", err)
		case ev, ok := <-watcher.Events:
			if !ok {
				return
			}
			id := m.eventPluginID(ev.Name)
			if id == "" {
				continue
			}
			// 顶层插件目录事件：维护子目录监听 + 触发注册/markMissing
			if m.isTopLevel(ev.Name) {
				if ev.Op&fsnotify.Create != 0 {
					if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() && !watched[ev.Name] {
						if err := watcher.Add(ev.Name); err != nil {
							m.logf("监听新插件目录失败 %s: %v", ev.Name, err)
						} else {
							watched[ev.Name] = true
						}
					}
					queue(id)
					continue
				}
				if ev.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
					if watched[ev.Name] {
						_ = watcher.Remove(ev.Name)
						delete(watched, ev.Name)
					}
					queue(id)
					continue
				}
				// 顶层目录 Chmod 等属性变化：忽略，避免误触发重启
				continue
			}
			// 子目录内：仅关键文件（manifest.json / plugin / plugin.exe）
			// 的变更代表升级，触发重启；static/* 等普通文件不重启。
			if m.isKeyFile(ev.Name) {
				queue(id)
			}
		case <-timerC:
			timer.Stop()
			timer = nil
			timerC = nil
			flush()
		}
	}
}

// isTopLevel 判断事件路径是否为根目录的直接子项（插件目录本身）。
func (m *Manager) isTopLevel(name string) bool {
	rel, err := filepath.Rel(m.dir, name)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return false
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	return len(parts) == 1 && parts[0] != ""
}

// isKeyFile 判断事件路径是否为插件目录下的关键文件
// （manifest.json 或可执行文件 plugin / plugin.exe）——仅这些文件的
// 就地替换代表"升级"，需重启进程；static/* 等其余文件变更不触发。
func (m *Manager) isKeyFile(name string) bool {
	rel, err := filepath.Rel(m.dir, name)
	if err != nil {
		return false
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	if len(parts) != 2 {
		return false
	}
	switch parts[1] {
	case "manifest.json", "plugin", "plugin.exe":
		return true
	}
	return false
}

// eventPluginID 从事件路径提取顶层插件目录名（根目录外的条目忽略）。
func (m *Manager) eventPluginID(name string) string {
	rel, err := filepath.Rel(m.dir, name)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return ""
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

// applyEvents 处理一批发生变动的插件目录。
func (m *Manager) applyEvents(ctx context.Context, ids []string) {
	for _, id := range ids {
		dir := filepath.Join(m.dir, id)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			// 目录被删除 → 停止进程，标记 missing（保留记录）
			m.markMissing(ctx, id)
			continue
		}
		bin, manifest, err := m.inspect(m.dir, id)
		if err != nil {
			// manifest 临时不完整（写入中间态）→ 去抖窗口已过仍不完整则告警
			m.logf("插件 %s 变更后校验失败: %v", id, err)
			continue
		}
		if err := m.upsert(manifest, bin); err != nil {
			m.logf("插件 %s 变更后记录同步失败: %v", id, err)
			continue
		}
		// 升级：停止旧进程再启动（manifest / 二进制被替换）
		m.stopProc(ctx, id, "upgrade")
		m.mu.Lock()
		proc, ok := m.procs[id]
		if !ok {
			proc = &Proc{
				id:       id,
				dir:      filepath.Join(m.dir, id),
				bin:      bin,
				manifest: manifest,
				status:   StatusStopped,
			}
			m.procs[id] = proc
		} else {
			proc.setBinManifest(bin, manifest)
		}
		m.mu.Unlock()
		p, err := m.repo.FindByID(id)
		if err != nil || p == nil {
			continue
		}
		if !p.IsActive {
			m.updateDBStatus(id, StatusStopped)
			continue
		}
		// stopProc 为防复活置了 stopping=true，升级是有意重启，须复位
		// 否则 spawn 的 stopping 守卫会静默拒绝，插件停在 stopped 不恢复。
		proc.setStopping(false)
		if err := m.spawn(ctx, proc); err != nil {
			m.logf("插件 %s 升级后启动失败: %v", id, err)
		}
	}
}
