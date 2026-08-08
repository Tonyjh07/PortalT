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

// watch 监听 PLUGINS_DIR 根目录的创建/删除/写入事件，驱动热加载：
//   - 新增目录 → inspect + upsert（新插件默认禁用，仅登记不 spawn）
//   - manifest / 二进制替换 → 重启插件（升级；先停再启）
//   - 目录删除 → 停止进程，DB 标记 missing
func (m *Manager) watch(ctx context.Context) {
	if m.Disabled() {
		return
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		m.logf("fsnotify 初始化失败: %v", err)
		return
	}
	defer watcher.Close()

	if err := watcher.Add(m.dir); err != nil {
		m.logf("监听插件目录失败 %s: %v", m.dir, err)
		return
	}
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
			if id := m.eventPluginID(ev.Name); id != "" {
				pending[id] = true
				if timer == nil {
					timer = time.NewTimer(watchDebounce)
					timerC = timer.C
				} else {
					// 重置去抖窗口
					timer.Reset(watchDebounce)
				}
			}
		case <-timerC:
			timer.Stop()
			timer = nil
			timerC = nil
			flush()
		}
	}
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
		if err := m.spawn(ctx, proc); err != nil {
			m.logf("插件 %s 升级后启动失败: %v", id, err)
		}
	}
}
