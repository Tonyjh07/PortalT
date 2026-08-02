package cron

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/plugins"
)

func setupRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("auth.user", &struct{}{})
		c.Next()
	})
	p := New()
	p.Mount(r.Group("/plugins/native/cron"), plugins.Deps{})
	return r
}

func get(t *testing.T, r *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func post(t *testing.T, r *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
	return w
}

func TestCron_ListJobs(t *testing.T) {
	r := setupRouter()
	w := get(t, r, "/plugins/native/cron/jobs")
	assert.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Data []Job `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data, 3)
	assert.Equal(t, "db-backup", body.Data[0].ID)
	assert.True(t, body.Data[0].Enabled)
	assert.Contains(t, body.Data[0].Schedule, "30")
	assert.NotNil(t, body.Data[0].NextRun, "任务应有下一次执行时间")
}

func TestCron_Toggle(t *testing.T) {
	r := setupRouter()
	w := post(t, r, "/plugins/native/cron/jobs/health-report/toggle")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"enabled":true`)

	// 重复切换 → 关闭
	w = post(t, r, "/plugins/native/cron/jobs/health-report/toggle")
	assert.Contains(t, w.Body.String(), `"enabled":false`)

	// 不存在的任务 → 404 + 4006
	w = post(t, r, "/plugins/native/cron/jobs/nope/toggle")
	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "4006")
}

func TestCron_RunAndLogs(t *testing.T) {
	r := setupRouter()

	w := post(t, r, "/plugins/native/cron/jobs/db-backup/run")
	assert.Equal(t, http.StatusOK, w.Code)
	var entry struct {
		Data LogEntry `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &entry))
	assert.Equal(t, "db-backup", entry.Data.JobID)
	assert.Equal(t, "ok", entry.Data.Status)
	assert.Contains(t, entry.Data.Detail, "备份")

	// 执行后 last_run 有值、run_count=1、日志出现
	w = get(t, r, "/plugins/native/cron/jobs")
	var jobs struct {
		Data []Job `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &jobs))
	db := jobs.Data[0]
	assert.NotNil(t, db.LastRun)
	assert.Equal(t, 1, db.RunCount)

	w = get(t, r, "/plugins/native/cron/logs")
	var logs struct {
		Data []LogEntry `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &logs))
	require.Len(t, logs.Data, 1)
	assert.Equal(t, "db-backup", logs.Data[0].JobID)

	// 不存在任务 → 404
	w = post(t, r, "/plugins/native/cron/jobs/nope/run")
	assert.Equal(t, http.StatusNotFound, w.Code)
}
