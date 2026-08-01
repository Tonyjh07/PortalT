package v1

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"portalt/internal/adapters/memory"
	"portalt/internal/api/middleware"
	"portalt/internal/domain"
)

// stubTokenManager 令牌管理器桩：固定返回指定用户。
type stubTokenManager struct {
	user *domain.User
}

func (s *stubTokenManager) ParseAccessToken(token string) (*domain.User, error) {
	if token == "" {
		return nil, errors.New("empty")
	}
	return s.user, nil
}

func (s *stubTokenManager) ParseRefreshToken(token string) (*domain.User, error) {
	return s.ParseAccessToken(token)
}

func (s *stubTokenManager) GenerateAccessToken(*domain.User) (string, error)  { return "a", nil }
func (s *stubTokenManager) GenerateRefreshToken(*domain.User) (string, error) { return "r", nil }
func (s *stubTokenManager) AccessTTL() time.Duration                          { return time.Minute }
func (s *stubTokenManager) RefreshTTL() time.Duration                         { return time.Hour }

func TestMenu_FiltersByPermission(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := memory.NewPluginRepository()
	require.NoError(t, repo.Save(&domain.Plugin{
		ID: "p-1", Name: "仪表盘", Route: "/", SortOrder: 1, IsActive: true,
	}))
	require.NoError(t, repo.Save(&domain.Plugin{
		ID: "p-2", Name: "用户管理", Route: "/users", SortOrder: 2, IsActive: true,
		Permission: domain.PERM_USER_MANAGE,
	}))
	require.NoError(t, repo.Save(&domain.Plugin{
		ID: "p-3", Name: "停用菜单", Route: "/disabled", SortOrder: 3, IsActive: false,
	}))

	handler := NewMenuHandler(repo)
	viewer := &domain.User{ID: "u-1", Role: domain.RoleViewer}

	r := gin.New()
	r.Use(middleware.AuthRequired(&stubTokenManager{user: viewer}))
	r.GET("/menu", handler.Menu)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/menu", nil)
	req.Header.Set("Authorization", "Bearer t")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Code int             `json:"code"`
		Data []domain.Plugin `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	assert.Equal(t, "p-1", body.Data[0].ID)
	assert.Equal(t, "/", body.Data[0].Route)
}

func TestMenu_AdminSeesAllActive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := memory.NewPluginRepository()
	require.NoError(t, repo.Save(&domain.Plugin{ID: "p-1", Name: "仪表盘", Route: "/", SortOrder: 1, IsActive: true}))
	require.NoError(t, repo.Save(&domain.Plugin{ID: "p-2", Name: "用户管理", Route: "/users", SortOrder: 2, IsActive: true,
		Permission: domain.PERM_USER_MANAGE}))

	handler := NewMenuHandler(repo)
	r := gin.New()
	r.Use(middleware.AuthRequired(&stubTokenManager{user: &domain.User{ID: "u-1", Role: domain.RoleAdmin}}))
	r.GET("/menu", handler.Menu)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/menu", nil)
	req.Header.Set("Authorization", "Bearer t")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Data []domain.Plugin `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Len(t, body.Data, 2)
}

func TestMenu_SortedBySortOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := memory.NewPluginRepository()
	require.NoError(t, repo.Save(&domain.Plugin{ID: "p-2", Name: "b", Route: "/b", SortOrder: 20, IsActive: true}))
	require.NoError(t, repo.Save(&domain.Plugin{ID: "p-1", Name: "a", Route: "/a", SortOrder: 10, IsActive: true}))

	handler := NewMenuHandler(repo)
	r := gin.New()
	r.Use(middleware.AuthRequired(&stubTokenManager{user: &domain.User{ID: "u-1", Role: domain.RoleAdmin}}))
	r.GET("/menu", handler.Menu)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/menu", nil)
	req.Header.Set("Authorization", "Bearer t")
	r.ServeHTTP(w, req)

	var body struct {
		Data []domain.Plugin `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data, 2)
	assert.Equal(t, "p-1", body.Data[0].ID)
	assert.Equal(t, "p-2", body.Data[1].ID)
}
