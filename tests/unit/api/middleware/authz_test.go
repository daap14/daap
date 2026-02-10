package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daap14/daap/internal/api/middleware"
	"github.com/daap14/daap/internal/auth"
	"github.com/daap14/daap/internal/team"
)

// createUserWithKey is a helper that creates a team+user and returns the raw API key.
func createUserWithKey(t *testing.T, svc *auth.Service, userRepo auth.UserRepository, teamRepo team.Repository, teamName, role string, isSuperuser bool) string {
	t.Helper()
	ctx := context.Background()

	var teamID *team.Team
	if !isSuperuser {
		tm := &team.Team{Name: teamName, Role: role}
		err := teamRepo.Create(ctx, tm)
		require.NoError(t, err)
		teamID = tm
	}

	rawKey, prefix, hash, err := svc.GenerateKey()
	require.NoError(t, err)

	u := &auth.User{
		Name:         teamName + "-user",
		IsSuperuser:  isSuperuser,
		ApiKeyPrefix: prefix,
		ApiKeyHash:   hash,
	}
	if teamID != nil {
		u.TeamID = &teamID.ID
	}

	err = userRepo.Create(ctx, u)
	require.NoError(t, err)

	return rawKey
}

// --- RequireSuperuser Tests ---

func TestRequireSuperuser_SuperuserAllowed(t *testing.T) {
	svc, userRepo, teamRepo, cleanup := setupAuthService(t)
	defer cleanup()

	rawKey := createUserWithKey(t, svc, userRepo, teamRepo, "su", "", true)

	handler := middleware.Auth(svc)(middleware.RequireSuperuser()(okHandler()))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", rawKey)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireSuperuser_NonSuperuserRejected(t *testing.T) {
	svc, userRepo, teamRepo, cleanup := setupAuthService(t)
	defer cleanup()

	rawKey := createUserWithKey(t, svc, userRepo, teamRepo, "normteam", "platform", false)

	handler := middleware.Auth(svc)(middleware.RequireSuperuser()(okHandler()))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", rawKey)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	env := parseErrorResponse(t, w)
	apiErr := env["error"].(map[string]interface{})
	assert.Equal(t, "FORBIDDEN", apiErr["code"])
	assert.Equal(t, "Superuser access required", apiErr["message"])
}

func TestRequireSuperuser_NoIdentity(t *testing.T) {
	// Call RequireSuperuser without Auth middleware (no identity in context)
	handler := middleware.RequireSuperuser()(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// --- RequireRole Tests ---

func TestRequireRole_PlatformUserAllowed(t *testing.T) {
	svc, userRepo, teamRepo, cleanup := setupAuthService(t)
	defer cleanup()

	rawKey := createUserWithKey(t, svc, userRepo, teamRepo, "platteam", "platform", false)

	handler := middleware.Auth(svc)(middleware.RequireRole("platform", "product")(okHandler()))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", rawKey)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireRole_ProductUserAllowed(t *testing.T) {
	svc, userRepo, teamRepo, cleanup := setupAuthService(t)
	defer cleanup()

	rawKey := createUserWithKey(t, svc, userRepo, teamRepo, "prodteam", "product", false)

	handler := middleware.Auth(svc)(middleware.RequireRole("platform", "product")(okHandler()))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", rawKey)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireRole_SuperuserRejected(t *testing.T) {
	svc, userRepo, teamRepo, cleanup := setupAuthService(t)
	defer cleanup()

	rawKey := createUserWithKey(t, svc, userRepo, teamRepo, "su-role", "", true)

	handler := middleware.Auth(svc)(middleware.RequireRole("platform", "product")(okHandler()))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", rawKey)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	env := parseErrorResponse(t, w)
	apiErr := env["error"].(map[string]interface{})
	assert.Equal(t, "FORBIDDEN", apiErr["code"])
	assert.Equal(t, "Insufficient permissions", apiErr["message"])
}

func TestRequireRole_WrongRoleRejected(t *testing.T) {
	svc, userRepo, teamRepo, cleanup := setupAuthService(t)
	defer cleanup()

	rawKey := createUserWithKey(t, svc, userRepo, teamRepo, "wrongrole", "product", false)

	// Only allow "platform", not "product"
	handler := middleware.Auth(svc)(middleware.RequireRole("platform")(okHandler()))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", rawKey)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	env := parseErrorResponse(t, w)
	apiErr := env["error"].(map[string]interface{})
	assert.Equal(t, "FORBIDDEN", apiErr["code"])
}

func TestRequireRole_NoIdentity(t *testing.T) {
	handler := middleware.RequireRole("platform")(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
