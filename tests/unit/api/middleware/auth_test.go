package middleware_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daap14/daap/internal/api/middleware"
	"github.com/daap14/daap/internal/auth"
	"github.com/daap14/daap/internal/team"
)

const defaultTestDatabaseURL = "postgres://daap:daap@127.0.0.1:5433/daap_test?sslmode=disable"
const testBcryptCost = 4

func setupAuthService(t *testing.T) (*auth.Service, auth.UserRepository, team.Repository, func()) {
	t.Helper()

	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = defaultTestDatabaseURL
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Skipf("skipping: cannot connect to test database: %v", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Skipf("skipping: cannot ping test database: %v", err)
	}

	_, err = pool.Exec(ctx, "TRUNCATE TABLE users CASCADE")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, "TRUNCATE TABLE teams CASCADE")
	require.NoError(t, err)

	userRepo := auth.NewRepository(pool)
	teamRepo := team.NewRepository(pool)
	svc := auth.NewService(userRepo, teamRepo, testBcryptCost)

	cleanup := func() { pool.Close() }
	return svc, userRepo, teamRepo, cleanup
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func parseErrorResponse(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err)
	return env
}

func TestAuth_MissingKey(t *testing.T) {
	svc, _, _, cleanup := setupAuthService(t)
	defer cleanup()

	handler := middleware.Auth(svc)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	env := parseErrorResponse(t, w)
	apiErr := env["error"].(map[string]interface{})
	assert.Equal(t, "UNAUTHORIZED", apiErr["code"])
	assert.Equal(t, "API key is required", apiErr["message"])
}

func TestAuth_EmptyKey(t *testing.T) {
	svc, _, _, cleanup := setupAuthService(t)
	defer cleanup()

	handler := middleware.Auth(svc)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	env := parseErrorResponse(t, w)
	apiErr := env["error"].(map[string]interface{})
	assert.Equal(t, "UNAUTHORIZED", apiErr["code"])
	assert.Equal(t, "API key is required", apiErr["message"])
}

func TestAuth_InvalidKey(t *testing.T) {
	svc, _, _, cleanup := setupAuthService(t)
	defer cleanup()

	handler := middleware.Auth(svc)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", "daap_invalidkeyvalue12345678901234567890")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	env := parseErrorResponse(t, w)
	apiErr := env["error"].(map[string]interface{})
	assert.Equal(t, "UNAUTHORIZED", apiErr["code"])
	assert.Equal(t, "Invalid or revoked API key", apiErr["message"])
}

func TestAuth_ValidKey_IdentityInContext(t *testing.T) {
	svc, userRepo, teamRepo, cleanup := setupAuthService(t)
	defer cleanup()

	ctx := context.Background()

	// Create team and user with a real key
	tm := &team.Team{Name: "middleware-team", Role: "platform"}
	err := teamRepo.Create(ctx, tm)
	require.NoError(t, err)

	rawKey, prefix, hash, err := svc.GenerateKey()
	require.NoError(t, err)

	u := &auth.User{
		Name:         "middleware-user",
		TeamID:       &tm.ID,
		IsSuperuser:  false,
		ApiKeyPrefix: prefix,
		ApiKeyHash:   hash,
	}
	err = userRepo.Create(ctx, u)
	require.NoError(t, err)

	var capturedIdentity *auth.Identity
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIdentity = middleware.GetIdentity(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.Auth(svc)(inner)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", rawKey)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, capturedIdentity)
	assert.Equal(t, u.ID, capturedIdentity.UserID)
	assert.Equal(t, "middleware-user", capturedIdentity.UserName)
	assert.Equal(t, "middleware-team", *capturedIdentity.TeamName)
	assert.Equal(t, "platform", *capturedIdentity.Role)
	assert.False(t, capturedIdentity.IsSuperuser)
}

func TestAuth_RevokedKey(t *testing.T) {
	svc, userRepo, teamRepo, cleanup := setupAuthService(t)
	defer cleanup()

	ctx := context.Background()

	tm := &team.Team{Name: "revoke-mw-team", Role: "product"}
	err := teamRepo.Create(ctx, tm)
	require.NoError(t, err)

	rawKey, prefix, hash, err := svc.GenerateKey()
	require.NoError(t, err)

	u := &auth.User{
		Name:         "revoked-mw-user",
		TeamID:       &tm.ID,
		IsSuperuser:  false,
		ApiKeyPrefix: prefix,
		ApiKeyHash:   hash,
	}
	err = userRepo.Create(ctx, u)
	require.NoError(t, err)

	err = userRepo.Revoke(ctx, u.ID)
	require.NoError(t, err)

	handler := middleware.Auth(svc)(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-API-Key", rawKey)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	env := parseErrorResponse(t, w)
	apiErr := env["error"].(map[string]interface{})
	assert.Equal(t, "UNAUTHORIZED", apiErr["code"])
	assert.Equal(t, "Invalid or revoked API key", apiErr["message"])
}

func TestGetIdentity_EmptyContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	identity := middleware.GetIdentity(req.Context())
	assert.Nil(t, identity)
}
