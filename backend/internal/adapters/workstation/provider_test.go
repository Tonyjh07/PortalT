package workstation

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/domain"
)

// vmrestHandler 模拟 vmrest API 的可编程处理器。
type vmrestHandler struct {
	username  string
	password  string
	vms       []map[string]any // GET /api/vms 返回的列表条目
	details   map[string]map[string]any
	host      map[string]any
	hostOK    bool
	lastOp    string // 最近一次电源操作 body
	opErrCode int    // 电源操作返回的错误状态码（0=成功）
}

func (h *vmrestHandler) serve() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/vms", func(w http.ResponseWriter, r *http.Request) {
		if !h.checkAuth(w, r) {
			return
		}
		_ = json.NewEncoder(w).Encode(h.vms)
	})
	mux.HandleFunc("/api/vms/", func(w http.ResponseWriter, r *http.Request) {
		if !h.checkAuth(w, r) {
			return
		}
		rest := strings.TrimPrefix(r.URL.Path, "/api/vms/")
		seg := strings.SplitN(rest, "/", 2)
		id := seg[0]
		if len(seg) > 1 && seg[1] == "power" {
			if r.Method == http.MethodPut {
				if h.opErrCode > 0 {
					w.WriteHeader(h.opErrCode)
					return
				}
				body := make([]byte, 64)
				n, _ := r.Body.Read(body)
				h.lastOp = strings.TrimSpace(string(body[:n]))
				_ = json.NewEncoder(w).Encode(map[string]string{"power_state": "poweredOn"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"power_state": "poweredOn"})
			return
		}
		d, ok := h.details[id]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(d)
	})
	mux.HandleFunc("/api/host", func(w http.ResponseWriter, r *http.Request) {
		if !h.checkAuth(w, r) {
			return
		}
		if !h.hostOK {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(h.host)
	})
	return httptest.NewServer(mux)
}

func (h *vmrestHandler) checkAuth(w http.ResponseWriter, r *http.Request) bool {
	u, p, ok := r.BasicAuth()
	if !ok || u != h.username || p != h.password {
		w.WriteHeader(http.StatusUnauthorized)
		return false
	}
	return true
}

func testProvider(t *testing.T, h *vmrestHandler) (*Provider, *httptest.Server) {
	t.Helper()
	srv := h.serve()
	p := NewProvider(Config{URL: srv.URL, Username: h.username, Password: h.password})
	return p, srv
}

func TestListVMs_Fields(t *testing.T) {
	h := &vmrestHandler{
		username: "u1",
		password: "p1",
		vms: []map[string]any{
			{"id": "VMID01", "path": `C:\VMs\ubuntu\ubuntu.vmx`},
			{"id": "VMID02", "path": `C:\VMs\win10\win10.vmx`},
		},
		details: map[string]map[string]any{
			"VMID01": {
				"name":           "ubuntu-dev",
				"power_state":    "on",
				"num_cpu":        4,
				"memory_size_MiB": float64(4096),
				"ip_address":     "192.168.88.10",
			},
			"VMID02": { // 老版本字段命名（子对象 + poweredOff）
				"power_state": "poweredOff",
				"cpu":         map[string]any{"processors": float64(2)},
				"memory":      map[string]any{"memory_MiB": float64(2048)},
			},
		},
	}
	p, srv := testProvider(t, h)
	defer srv.Close()

	vms, err := p.ListVMs()
	require.NoError(t, err)
	require.Len(t, vms, 2)

	vm1 := vms[0]
	assert.Equal(t, "VMID01", vm1.ID)
	assert.Equal(t, "ubuntu-dev", vm1.Name)
	assert.Equal(t, domain.VMStatusPoweredOn, vm1.Status)
	assert.Equal(t, 4, vm1.CPU)
	assert.Equal(t, 4096, vm1.MemoryMB)
	assert.Equal(t, "192.168.88.10", vm1.IPAddress)
	assert.Equal(t, "192.168.88.10", vm1.Metadata["guac.hostname"])

	vm2 := vms[1]
	assert.Equal(t, "win10", vm2.Name) // 无 name 时回退 vmx 文件名
	assert.Equal(t, domain.VMStatusPoweredOff, vm2.Status)
	assert.Equal(t, 2, vm2.CPU)
	assert.Equal(t, 2048, vm2.MemoryMB)
	assert.Empty(t, vm2.IPAddress)
}

func TestListVMs_AuthAndStatusVariants(t *testing.T) {
	h := &vmrestHandler{
		username: "u1",
		password: "p1",
		vms: []map[string]any{
			{"id": "A", "path": "a.vmx"},
			{"id": "B", "path": "b.vmx"},
			{"id": "C", "path": "c.vmx"},
		},
		details: map[string]map[string]any{
			"A": {"power_state": "suspended"},
			"B": {"power_state": "weird-state"},
			"C": {"power_state": "poweredOn", "name": "c"},
		},
	}
	p, srv := testProvider(t, h)
	defer srv.Close()

	vms, err := p.ListVMs()
	require.NoError(t, err)
	assert.Equal(t, domain.VMStatusSuspended, vms[0].Status)
	assert.Equal(t, domain.VMStatusUnknown, vms[1].Status)
	assert.Equal(t, domain.VMStatusPoweredOn, vms[2].Status)
}

func TestListVMs_Unauthorized(t *testing.T) {
	h := &vmrestHandler{username: "u1", password: "p1"}
	p, srv := testProvider(t, h)
	defer srv.Close()
	p.cfg.Password = "wrong"

	_, err := p.ListVMs()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

func TestListVMs_ConnectionRefused(t *testing.T) {
	p := NewProvider(Config{URL: "http://127.0.0.1:1", Username: "u", Password: "p", Timeout: 500 * time.Millisecond})
	_, err := p.ListVMs()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "请求 /api/vms 失败")
}

func TestPowerOps(t *testing.T) {
	h := &vmrestHandler{username: "u1", password: "p1"}
	p, srv := testProvider(t, h)
	defer srv.Close()

	require.NoError(t, p.StartVM("VMID01"))
	assert.Equal(t, "on", h.lastOp)
	require.NoError(t, p.StopVM("VMID01"))
	assert.Equal(t, "off", h.lastOp)
	require.NoError(t, p.RestartVM("VMID01"))
	assert.Equal(t, "reset", h.lastOp)
}

func TestPowerOp_Failure(t *testing.T) {
	h := &vmrestHandler{username: "u1", password: "p1", opErrCode: http.StatusNotFound}
	p, srv := testProvider(t, h)
	defer srv.Close()

	err := p.StartVM("VMID01")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "404")
}

func TestGetHostInfo_WithAPI(t *testing.T) {
	h := &vmrestHandler{
		username: "u1",
		password: "p1",
		hostOK:   true,
		host: map[string]any{
			"host_name":    "dev-host",
			"version":      "17.0.0",
			"cpu_total":    float64(8),
			"cpu_used":     float64(2),
			"memory_total": float64(32768),
			"memory_used":  float64(12288),
		},
	}
	p, srv := testProvider(t, h)
	defer srv.Close()

	info, err := p.GetHostInfo()
	require.NoError(t, err)
	assert.Equal(t, "dev-host", info.Name)
	assert.Equal(t, "17.0.0", info.Version)
	assert.Equal(t, 8, info.TotalCPU)
	assert.Equal(t, 2, info.UsedCPU)
	assert.Equal(t, 32768, info.TotalMemoryMB)
	assert.Equal(t, 12288, info.UsedMemoryMB)
	assert.Equal(t, "connected", info.Status)
	assert.InDelta(t, 25.0, info.CPUUsagePercent(), 0.01)
}

func TestGetHostInfo_Fallback(t *testing.T) {
	h := &vmrestHandler{username: "u1", password: "p1", hostOK: false}
	p, srv := testProvider(t, h)
	defer srv.Close()

	info, err := p.GetHostInfo()
	require.NoError(t, err)
	assert.NotEmpty(t, info.Name)
	assert.Equal(t, "connected", info.Status)
	assert.Equal(t, 0, info.TotalCPU)
}

func TestBaseName(t *testing.T) {
	assert.Equal(t, "ubuntu", baseName(`C:\VMs\ubuntu\ubuntu.vmx`))
	assert.Equal(t, "win10", baseName(`C:\VMs\win10\win10.vmx`))
	assert.Equal(t, "a", baseName(`/home/user/a.vmx`))
	assert.Equal(t, "b", baseName("b.vmx"))
}
