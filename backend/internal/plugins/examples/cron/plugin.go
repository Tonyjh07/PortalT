// Package cron 是 PortalT 原生插件机制的示例插件。
//
// 演示能力：常驻后台调度器（goroutine）+ 自定义 API + 内嵌静态前端。
// 提供内存版定时任务演示：任务按分钟间隔调度，可启用/禁用、立即执行、
// 查看执行日志。注意：任务与日志仅存内存，重启后端后重置（示例不持久化）。
package cron

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"portalt/internal/api/response"
	"portalt/internal/domain"
	"portalt/internal/plugins"
)

//go:embed static/*
var staticFiles embed.FS

// Job 定时任务。
type Job struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Desc        string     `json:"desc"`
	Schedule    string     `json:"schedule"`
	IntervalMin int        `json:"interval_min"`
	Enabled     bool       `json:"enabled"`
	LastRun     *time.Time `json:"last_run"`
	NextRun     *time.Time `json:"next_run"`
	RunCount    int        `json:"run_count"`
}

// LogEntry 执行日志。
type LogEntry struct {
	Time    time.Time `json:"time"`
	JobID   string    `json:"job_id"`
	JobName string    `json:"job_name"`
	Status  string    `json:"status"`
	Detail  string    `json:"detail"`
}

var errNotFound = fmt.Errorf("任务不存在")

// scheduler 内存调度器（示例实现，重启即重置）。
type scheduler struct {
	mu   sync.Mutex
	jobs []*Job
	logs []LogEntry
}

// newScheduler 创建调度器并内置示例任务，随后启动后台调度循环。
func newScheduler() *scheduler {
	now := time.Now()
	s := &scheduler{jobs: []*Job{
		{ID: "db-backup", Name: "数据库备份", Desc: "导出 PostgreSQL 全量备份到 /data/backups", IntervalMin: 30, Enabled: true},
		{ID: "log-clean", Name: "清理过期日志", Desc: "删除 7 天前的 Nginx 与应用日志", IntervalMin: 15, Enabled: true},
		{ID: "health-report", Name: "健康检查报告", Desc: "汇总宿主机与 VM 状态生成日报", IntervalMin: 60, Enabled: false},
	}}
	for _, j := range s.jobs {
		j.Schedule = fmt.Sprintf("每 %d 分钟", j.IntervalMin)
		next := now.Add(time.Duration(j.IntervalMin) * time.Minute)
		j.NextRun = &next
	}
	go s.loop()
	return s
}

// loop 调度主循环：每 20 秒检查一次到期的启用任务。
func (s *scheduler) loop() {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		s.mu.Lock()
		for _, j := range s.jobs {
			if j.Enabled && j.NextRun != nil && !j.NextRun.After(now) {
				s.executeLocked(j, now, "定时触发")
			}
		}
		s.mu.Unlock()
	}
}

// run 立即执行任务（禁用状态也可手动执行），返回本次日志。
func (s *scheduler) run(id, source string) (*LogEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.jobs {
		if j.ID == id {
			return s.executeLocked(j, time.Now(), source), nil
		}
	}
	return nil, errNotFound
}

// toggle 切换任务启用状态；重新启用时若上次排程已过期则重新排程。
func (s *scheduler) toggle(id string) (*Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.jobs {
		if j.ID == id {
			j.Enabled = !j.Enabled
			if j.Enabled {
				now := time.Now()
				if j.NextRun == nil || j.NextRun.Before(now) {
					next := now.Add(time.Duration(j.IntervalMin) * time.Minute)
					j.NextRun = &next
				}
			}
			return j, nil
		}
	}
	return nil, errNotFound
}

// executeLocked 模拟执行任务（调用方须持锁）。
func (s *scheduler) executeLocked(j *Job, at time.Time, source string) *LogEntry {
	j.LastRun = &at
	next := at.Add(time.Duration(j.IntervalMin) * time.Minute)
	j.NextRun = &next
	j.RunCount++
	detail := simulateDetail(j.ID, at)
	entry := LogEntry{Time: at, JobID: j.ID, JobName: j.Name, Status: "ok", Detail: source + "：" + detail}
	s.logs = append(s.logs, entry)
	if len(s.logs) > 50 {
		s.logs = s.logs[len(s.logs)-50:]
	}
	return &entry
}

// simulateDetail 生成模拟执行结果文案。
func simulateDetail(id string, at time.Time) string {
	switch id {
	case "db-backup":
		return fmt.Sprintf("备份完成，生成 %s.sql（12.4 MB，耗时 8s）", at.Format("20060102_150405"))
	case "log-clean":
		return "清理 1,382 个过期日志文件，释放 96 MB"
	case "health-report":
		return "报告已生成：宿主机正常，8/8 台 VM 在线"
	default:
		return "模拟执行完成"
	}
}

// snapshot 返回任务列表副本（供 handler 安全序列化）。
func (s *scheduler) snapshot() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, *j)
	}
	return out
}

// recentLogs 返回最近日志（新→旧）。
func (s *scheduler) recentLogs() []LogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]LogEntry, len(s.logs))
	for i := range s.logs {
		out[len(s.logs)-1-i] = s.logs[i]
	}
	return out
}

// Plugin cron 原生插件。
type Plugin struct {
	s *scheduler
}

// New 创建插件实例（启动内置调度器）。
func New() *Plugin { return &Plugin{s: newScheduler()} }

// Info 插件元信息（菜单/权限同步用）。
func (p *Plugin) Info() domain.Plugin {
	return domain.Plugin{
		ID:        "cron",
		Name:      "定时任务",
		Icon:      "mdi:calendar-clock",
		Route:     "/cron",
		SortOrder: 91,
		IsActive:  true,
	}
}

// Mount 挂载插件 API 路由（已位于 /api/v1/plugins/native/cron/ 下）。
func (p *Plugin) Mount(rt *gin.RouterGroup, _ plugins.Deps) {
	rt.GET("/jobs", p.listJobs)
	rt.POST("/jobs/:id/toggle", p.toggleJob)
	rt.POST("/jobs/:id/run", p.runJob)
	rt.GET("/logs", p.listLogs)
}

// StaticFS 返回内嵌静态前端（托管于 /native/cron/）。
func (p *Plugin) StaticFS() fs.FS {
	sub, err := fs.Sub(staticFiles, "static")
	if err != nil {
		return nil
	}
	return sub
}

func (p *Plugin) listJobs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"data": p.s.snapshot()})
}

func (p *Plugin) toggleJob(c *gin.Context) {
	job, err := p.s.toggle(strings.TrimSpace(c.Param("id")))
	if err != nil {
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "任务不存在")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": job})
}

func (p *Plugin) runJob(c *gin.Context) {
	entry, err := p.s.run(strings.TrimSpace(c.Param("id")), "手动触发")
	if err != nil {
		response.Error(c, http.StatusNotFound, response.CodeNotFound, "任务不存在")
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": entry})
}

func (p *Plugin) listLogs(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"data": p.s.recentLogs()})
}
