package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daap14/daap/internal/api"
	"github.com/daap14/daap/internal/auth"
	"github.com/daap14/daap/internal/database"
	"github.com/daap14/daap/internal/k8s"
	"github.com/daap14/daap/internal/team"
	"github.com/daap14/daap/internal/tier"
)

// authTestEnv holds the test server and auth-related objects for auth integration tests.
type authTestEnv struct {
	server      *httptest.Server
	authService *auth.Service
	teamRepo    team.Repository
	userRepo    auth.UserRepository
	superKey    string
}

func setupAuthTestServer(t *testing.T) *authTestEnv {
	t.Helper()

	if testPool == nil {
		t.Skip("skipping: test database not available")
	}

	ctx := context.Background()

	// Truncate for clean slate (order matters due to FK constraints)
	_, err := testPool.Exec(ctx, "TRUNCATE TABLE databases CASCADE")
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, "TRUNCATE TABLE tiers CASCADE")
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, "TRUNCATE TABLE users CASCADE")
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, "TRUNCATE TABLE teams CASCADE")
	require.NoError(t, err)

	repo := database.NewRepository(testPool)
	mgr := &dbMockManager{}
	teamRepo := team.NewRepository(testPool)
	tierRepo := tier.NewPostgresRepository(testPool)
	userRepo := auth.NewRepository(testPool)
	authService := auth.NewService(userRepo, teamRepo, 4)

	// Bootstrap superuser
	superKey, err := authService.BootstrapSuperuser(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, superKey)

	checker := &mockHealthChecker{
		status: k8s.ConnectivityStatus{Connected: true, Version: "v1.31.0"},
	}
	pinger := &dbTestPinger{pool: testPool}

	router := api.NewRouter(api.RouterDeps{
		K8sChecker:  checker,
		DBPinger:    pinger,
		Version:     "0.1.0-test",
		Repo:        repo,
		K8sManager:  mgr,
		Namespace:   "default",
		AuthService: authService,
		TeamRepo:    teamRepo,
		TierRepo:    tierRepo,
		UserRepo:    userRepo,
	})

	server := httptest.NewServer(router)
	t.Cleanup(func() { server.Close() })

	return &authTestEnv{
		server:      server,
		authService: authService,
		teamRepo:    teamRepo,
		userRepo:    userRepo,
		superKey:    superKey,
	}
}

// authDoRequest is a helper for auth tests — reuses dbDoRequest.
func authDoRequest(t *testing.T, method, url string, body interface{}, apiKey string) (*http.Response, map[string]interface{}) {
	t.Helper()
	return dbDoRequest(t, method, url, body, apiKey)
}

// ===== Auth Lifecycle Test =====

func TestAuthLifecycle(t *testing.T) {
	env := setupAuthTestServer(t)

	// Step 1: No API key -> 401 on authenticated routes
	t.Run("no API key returns 401", func(t *testing.T) {
		resp, result := authDoRequest(t, http.MethodGet, env.server.URL+"/teams", nil, "")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		errObj := result["error"].(map[string]interface{})
		assert.Equal(t, "UNAUTHORIZED", errObj["code"])
	})

	// Step 2: Invalid API key -> 401
	t.Run("invalid API key returns 401", func(t *testing.T) {
		resp, result := authDoRequest(t, http.MethodGet, env.server.URL+"/teams", nil, "daap_invalidkey12345678901234567890")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		errObj := result["error"].(map[string]interface{})
		assert.Equal(t, "UNAUTHORIZED", errObj["code"])
	})

	// Step 3: Superuser can create a team
	var teamID string
	t.Run("superuser creates team", func(t *testing.T) {
		body := map[string]interface{}{
			"name": "test-team",
			"role": "platform",
		}
		resp, result := authDoRequest(t, http.MethodPost, env.server.URL+"/teams", body, env.superKey)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		data := result["data"].(map[string]interface{})
		teamID = data["id"].(string)
		assert.NotEmpty(t, teamID)
		assert.Equal(t, "test-team", data["name"])
		assert.Equal(t, "platform", data["role"])
	})

	// Step 4: Superuser can list teams
	t.Run("superuser lists teams", func(t *testing.T) {
		resp, result := authDoRequest(t, http.MethodGet, env.server.URL+"/teams", nil, env.superKey)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		// List includes at least the team we just created
		listData := result["data"].([]interface{})
		assert.GreaterOrEqual(t, len(listData), 1)
	})

	// Step 5: Superuser creates a user in the team
	var userAPIKey string
	t.Run("superuser creates user", func(t *testing.T) {
		body := map[string]interface{}{
			"name":   "test-user",
			"teamId": teamID,
		}
		resp, result := authDoRequest(t, http.MethodPost, env.server.URL+"/users", body, env.superKey)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		data := result["data"].(map[string]interface{})
		assert.NotEmpty(t, data["id"])
		assert.Equal(t, "test-user", data["name"])
		assert.Equal(t, teamID, data["teamId"])
		assert.Equal(t, "test-team", data["teamName"])
		assert.Equal(t, "platform", data["role"])
		userAPIKey = data["apiKey"].(string)
		assert.NotEmpty(t, userAPIKey)
	})

	// Step 6: User can authenticate and access database routes
	t.Run("user accesses databases", func(t *testing.T) {
		resp, result := authDoRequest(t, http.MethodGet, env.server.URL+"/databases", nil, userAPIKey)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		listData := result["data"].([]interface{})
		assert.Len(t, listData, 0) // no databases yet, but access granted
	})

	// Step 7: User cannot access superuser-only routes
	t.Run("user cannot access teams (superuser-only)", func(t *testing.T) {
		resp, result := authDoRequest(t, http.MethodGet, env.server.URL+"/teams", nil, userAPIKey)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		errObj := result["error"].(map[string]interface{})
		assert.Equal(t, "FORBIDDEN", errObj["code"])
	})

	// Step 8: Superuser lists users
	var userID string
	t.Run("superuser lists users", func(t *testing.T) {
		resp, result := authDoRequest(t, http.MethodGet, env.server.URL+"/users", nil, env.superKey)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		listData := result["data"].([]interface{})
		assert.GreaterOrEqual(t, len(listData), 2) // superuser + test-user
		// Find our test user
		for _, item := range listData {
			u := item.(map[string]interface{})
			if u["name"] == "test-user" {
				userID = u["id"].(string)
			}
		}
		require.NotEmpty(t, userID)
	})

	// Step 9: Revoke the user
	t.Run("superuser revokes user", func(t *testing.T) {
		resp, _ := authDoRequest(t, http.MethodDelete, env.server.URL+"/users/"+userID, nil, env.superKey)
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	// Step 10: Revoked user's key is rejected
	t.Run("revoked user key returns 401", func(t *testing.T) {
		resp, result := authDoRequest(t, http.MethodGet, env.server.URL+"/databases", nil, userAPIKey)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		errObj := result["error"].(map[string]interface{})
		assert.Equal(t, "UNAUTHORIZED", errObj["code"])
	})
}

// ===== Superuser Bootstrap =====

func TestSuperuserBootstrap_Idempotent(t *testing.T) {
	env := setupAuthTestServer(t)

	// BootstrapSuperuser again should return empty string (already exists)
	rawKey, err := env.authService.BootstrapSuperuser(context.Background())
	require.NoError(t, err)
	assert.Empty(t, rawKey, "second bootstrap should be a no-op")

	// Original key still works
	resp, _ := authDoRequest(t, http.MethodGet, env.server.URL+"/teams", nil, env.superKey)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

// ===== Superuser Cannot Be Revoked =====

func TestSuperuserCannotBeRevoked(t *testing.T) {
	env := setupAuthTestServer(t)

	// Find the superuser ID
	resp, result := authDoRequest(t, http.MethodGet, env.server.URL+"/users", nil, env.superKey)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	listData := result["data"].([]interface{})
	var superID string
	for _, item := range listData {
		u := item.(map[string]interface{})
		if u["isSuperuser"] == true {
			superID = u["id"].(string)
		}
	}
	require.NotEmpty(t, superID)

	// Attempt to revoke superuser -> 403
	resp, result = authDoRequest(t, http.MethodDelete, env.server.URL+"/users/"+superID, nil, env.superKey)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "FORBIDDEN", errObj["code"])
}

// ===== Team CRUD Integration =====

func TestTeamCRUD(t *testing.T) {
	env := setupAuthTestServer(t)

	// Create team
	var teamID string
	t.Run("create team", func(t *testing.T) {
		body := map[string]interface{}{
			"name": "crud-team",
			"role": "product",
		}
		resp, result := authDoRequest(t, http.MethodPost, env.server.URL+"/teams", body, env.superKey)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		data := result["data"].(map[string]interface{})
		teamID = data["id"].(string)
		assert.Equal(t, "crud-team", data["name"])
		assert.Equal(t, "product", data["role"])
	})

	// Duplicate name -> 409
	t.Run("duplicate team name", func(t *testing.T) {
		body := map[string]interface{}{
			"name": "crud-team",
			"role": "platform",
		}
		resp, result := authDoRequest(t, http.MethodPost, env.server.URL+"/teams", body, env.superKey)
		assert.Equal(t, http.StatusConflict, resp.StatusCode)
		errObj := result["error"].(map[string]interface{})
		assert.Equal(t, "DUPLICATE_NAME", errObj["code"])
	})

	// Delete team
	t.Run("delete team", func(t *testing.T) {
		resp, _ := authDoRequest(t, http.MethodDelete, env.server.URL+"/teams/"+teamID, nil, env.superKey)
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	// Delete again -> 404
	t.Run("delete nonexistent team", func(t *testing.T) {
		resp, result := authDoRequest(t, http.MethodDelete, env.server.URL+"/teams/"+teamID, nil, env.superKey)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		errObj := result["error"].(map[string]interface{})
		assert.Equal(t, "NOT_FOUND", errObj["code"])
	})
}

// ===== Team Deletion with Users =====

func TestTeamDeleteWithUsers(t *testing.T) {
	env := setupAuthTestServer(t)

	// Create team
	body := map[string]interface{}{
		"name": "team-with-users",
		"role": "platform",
	}
	resp, result := authDoRequest(t, http.MethodPost, env.server.URL+"/teams", body, env.superKey)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	teamID := result["data"].(map[string]interface{})["id"].(string)

	// Create user in team
	userBody := map[string]interface{}{
		"name":   "user-in-team",
		"teamId": teamID,
	}
	resp, _ = authDoRequest(t, http.MethodPost, env.server.URL+"/users", userBody, env.superKey)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Delete team -> 409 (has users)
	resp, result = authDoRequest(t, http.MethodDelete, env.server.URL+"/teams/"+teamID, nil, env.superKey)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "TEAM_HAS_USERS", errObj["code"])
}

// ===== Public Routes Do Not Require Auth =====

func TestPublicRoutesNoAuth(t *testing.T) {
	env := setupAuthTestServer(t)

	// Health endpoint is public — no API key needed
	resp, result := authDoRequest(t, http.MethodGet, env.server.URL+"/health", nil, "")
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	data := result["data"].(map[string]interface{})
	assert.NotNil(t, data["status"])
}

// ===== Superuser Cannot Access Business Routes =====

func TestSuperuserCannotAccessBusinessRoutes(t *testing.T) {
	env := setupAuthTestServer(t)

	// Superuser has no role, so RequireRole("platform","product") rejects
	resp, result := authDoRequest(t, http.MethodGet, env.server.URL+"/databases", nil, env.superKey)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "FORBIDDEN", errObj["code"])
}
