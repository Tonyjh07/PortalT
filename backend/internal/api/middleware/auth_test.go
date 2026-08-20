package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"portalt/internal/domain"
)

// stubTokenManager 可编程的令牌管理器桩。
type stubTokenManager struct {
	user *domain.User
	err  error
}

func (s *stubTokenManager) ParseAccessToken(token string) (*domain.User, error) {
	if s.err != nil {
		return nil, s.err
	}
	if token == "" {
		return nil, errors.New("empty")
	}
	return s.user, nil
}

func (s *stubTokenManager) ParseRefreshToken(token string) (*domain.User, error) {
	return s.ParseAccessToken(token)
}

func (s *stubTokenManager) GenerateAccessToken(*domain.User) (string, error) {
	return "access", nil
}

func (s *stubTokenManager) GenerateRefreshToken(*domain.User) (string, error) {
	return "refresh", nil
}

func (s *stubTokenManager) AccessTTL() time.Duration  { return time.Minute }
func (s *stubTokenManager) RefreshTTL() time.Duration { return 24 * time.Hour }

func setupGin() (*gin.Engine, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	return gin.New(), httptest.NewRecorder()
}

func TestAuthRequired_ValidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r, w := setupGin()

	tm := &stubTokenManager{user: &domain.User{ID: "u-1", Username: "alice", Role: domain.RoleAdmin}}
	r.GET("/protected", AuthRequired(tm), func(c *gin.Context) {
		u := CurrentUser(c)
		assert.Equal(t, "alice", u.Username)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthRequired_MissingHeader(t *testing.T) {
	r, w := setupGin()
	tm := &stubTokenManager{}

	r.GET("/protected", AuthRequired(tm), func(c *gin.Context) {
		t.Fatal("不应进入处理器")
	})

	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/protected", nil))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "缺少访问令牌")
}

func TestAuthRequired_BadScheme(t *testing.T) {
	r, w := setupGin()
	tm := &stubTokenManager{}

	r.GET("/protected", AuthRequired(tm), func(c *gin.Context) {
		t.Fatal("不应进入处理器")
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthRequired_EmptyToken(t *testing.T) {
	r, w := setupGin()
	tm := &stubTokenManager{}

	r.GET("/protected", AuthRequired(tm), func(c *gin.Context) {
		t.Fatal("不应进入处理器")
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer ")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuthRequired_InvalidToken(t *testing.T) {
	r, w := setupGin()
	tm := &stubTokenManager{err: errors.New("expired")}

	r.GET("/protected", AuthRequired(tm), func(c *gin.Context) {
		t.Fatal("不应进入处理器")
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer bad-token")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "无效或已过期")
}

func TestAuthRequired_QueryToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r, w := setupGin()

	tm := &stubTokenManager{user: &domain.User{ID: "u-1", Username: "bob", Role: domain.RoleUser}}
	r.GET("/ws", AuthRequired(tm), func(c *gin.Context) {
		u := CurrentUser(c)
		assert.Equal(t, "bob", u.Username)
		c.Status(http.StatusOK)
	})

	// WebSocket 升级请求无法携带自定义头，支持 ?token= 回退
	req := httptest.NewRequest(http.MethodGet, "/ws?token=valid-token", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuthRequired_QueryToken_GuacamoleAppendedData(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r, _ := setupGin()

	tm := &stubTokenManager{user: &domain.User{ID: "u-1", Username: "bob", Role: domain.RoleUser}}
	r.GET("/ws", AuthRequired(tm), func(c *gin.Context) {
		u := CurrentUser(c)
		assert.Equal(t, "bob", u.Username)
		c.Status(http.StatusOK)
	})

	// guacamole-common-js 的 WebSocketTunnel 会追加 "?" + connect 数据
	// （无参时为 "undefined"），token 后出现 ? 或 & 均应截断
	for _, q := range []string{
		"/ws?token=valid-token?undefined",
		"/ws?token=valid-token&connect=abc",
	} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, q, nil))
		assert.Equal(t, http.StatusOK, w.Code, "query=%s", q)
	}
}

func TestCurrentUser_NilWhenUnauthenticated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r, w := setupGin()

	r.GET("/", func(c *gin.Context) {
		assert.Nil(t, CurrentUser(c))
		c.Status(http.StatusOK)
	})
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	assert.Equal(t, http.StatusOK, w.Code)
}
