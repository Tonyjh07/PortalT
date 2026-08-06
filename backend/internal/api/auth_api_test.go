package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authadapter "portalt/internal/adapters/auth"
	"portalt/internal/adapters/memory"
	"portalt/internal/adapters/mock"
	"portalt/internal/api/v1"
	"portalt/internal/domain/services"
	"portalt/internal/ports"
)

// testEnv 组装完整的 API 环境（真实仓储 + 认证 + JWT + 虚拟机服务）。
type testEnv struct {
	router   *gin.Engine
	userRepo *memory.UserRepository
	vmRepo   *memory.VMRepository
	plugins  *memory.PluginRepository
	provider *mock.Provider
	tm       ports.TokenManager
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	userRepo := memory.NewUserRepository()
	require.NoError(t, authadapter.EnsureAdminUser(t.Context(), userRepo, "admin", "admin123"))

	authProvider := authadapter.NewLocalProvider(userRepo)
	tm := authadapter.NewJWTManager("test-secret", 15*time.Minute, 7*24*time.Hour)

	provider := mock.NewProvider(nil)
	vmRepo := memory.NewVMRepository()
	plugins := memory.NewPluginRepository()

	router := NewRouter(tm, &HandlerSet{
		Auth:   v1.NewAuthHandler(authProvider, tm),
		VM:     v1.NewVMHandler(services.NewVMService(vmRepo, provider), nil),
		Menu:   v1.NewMenuHandler(plugins),
		Plugin: v1.NewPluginHandler(plugins, nil),
		Guac:   v1.NewGuacHandler(""),
	})
	return &testEnv{router: router, userRepo: userRepo, vmRepo: vmRepo, plugins: plugins, provider: provider, tm: tm}
}

// loginAndToken 登录管理员并返回访问令牌。
func loginAndToken(t *testing.T, env *testEnv) string {
	t.Helper()
	_, body := login(t, env.router, "admin", "admin123")
	token := body["data"].(map[string]any)["access_token"].(string)
	require.NotEmpty(t, token)
	return token
}

// doRequest 执行 HTTP 请求并返回响应。
func doRequest(t *testing.T, router *gin.Engine, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// login 登录并返回响应体。
func login(t *testing.T, router *gin.Engine, username, password string) (int, map[string]any) {
	t.Helper()
	w := doRequest(t, router, http.MethodPost, "/api/v1/auth/login",
		map[string]string{"username": username, "password": password}, "")
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return w.Code, body
}

func TestLogin_Success(t *testing.T) {
	env := setupTestEnv(t)

	code, body := login(t, env.router, "admin", "admin123")
	require.Equal(t, http.StatusOK, code)

	data := body["data"].(map[string]any)
	access := data["access_token"].(string)
	refresh := data["refresh_token"].(string)
	assert.NotEmpty(t, access)
	assert.NotEmpty(t, refresh)
	assert.NotEqual(t, access, refresh, "两种令牌应不同")

	user := data["user"].(map[string]any)
	assert.Equal(t, "admin", user["username"])
	assert.Equal(t, "admin", user["role"])
	assert.NotContains(t, user, "password", "响应不应暴露密码")
}

func TestLogin_WrongPassword(t *testing.T) {
	env := setupTestEnv(t)

	code, body := login(t, env.router, "admin", "wrong")
	assert.Equal(t, http.StatusUnauthorized, code)
	assert.Equal(t, float64(4001), body["code"])
}

func TestLogin_UnknownUser(t *testing.T) {
	env := setupTestEnv(t)

	code, body := login(t, env.router, "ghost", "admin123")
	assert.Equal(t, http.StatusUnauthorized, code)
	assert.Equal(t, float64(4001), body["code"])
}

func TestLogin_BadRequest(t *testing.T) {
	env := setupTestEnv(t)

	w := doRequest(t, env.router, http.MethodPost, "/api/v1/auth/login", map[string]string{}, "")
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = doRequest(t, env.router, http.MethodPost, "/api/v1/auth/login", "not-json", "")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRefresh_Success(t *testing.T) {
	env := setupTestEnv(t)

	_, body := login(t, env.router, "admin", "admin123")
	refresh := body["data"].(map[string]any)["refresh_token"].(string)

	w := doRequest(t, env.router, http.MethodPost, "/api/v1/auth/refresh",
		map[string]string{"refresh_token": refresh}, "")
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	newAccess := resp["data"].(map[string]any)["access_token"].(string)
	assert.NotEmpty(t, newAccess)
}

func TestRefresh_WithAccessToken_Fails(t *testing.T) {
	env := setupTestEnv(t)

	_, body := login(t, env.router, "admin", "admin123")
	access := body["data"].(map[string]any)["access_token"].(string)

	w := doRequest(t, env.router, http.MethodPost, "/api/v1/auth/refresh",
		map[string]string{"refresh_token": access}, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, float64(4002), unmarshalCode(t, w))
}

func TestRefresh_GarbageToken(t *testing.T) {
	env := setupTestEnv(t)

	w := doRequest(t, env.router, http.MethodPost, "/api/v1/auth/refresh",
		map[string]string{"refresh_token": "garbage"}, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMe_WithToken(t *testing.T) {
	env := setupTestEnv(t)

	_, body := login(t, env.router, "admin", "admin123")
	access := body["data"].(map[string]any)["access_token"].(string)

	w := doRequest(t, env.router, http.MethodGet, "/api/v1/auth/me", nil, access)
	require.Equal(t, http.StatusOK, w.Code)

	var resp map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	user := resp["data"].(map[string]any)
	assert.Equal(t, "admin", user["username"])
	assert.Equal(t, "admin", user["role"])
}

func TestMe_WithoutToken(t *testing.T) {
	env := setupTestEnv(t)

	w := doRequest(t, env.router, http.MethodGet, "/api/v1/auth/me", nil, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, float64(4003), unmarshalCode(t, w))
}

func unmarshalCode(t *testing.T, w *httptest.ResponseRecorder) float64 {
	t.Helper()
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body["code"].(float64)
}

func TestHealthz(t *testing.T) {
	env := setupTestEnv(t)

	w := doRequest(t, env.router, http.MethodGet, "/healthz", nil, "")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "PortalT")
}
