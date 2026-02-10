package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daap14/daap/internal/api"
	"github.com/daap14/daap/internal/auth"
	"github.com/daap14/daap/internal/database"
	"github.com/daap14/daap/internal/k8s"
	"github.com/daap14/daap/internal/team"
)

// ownerTestEnv holds all the keys and IDs needed for ownership tests.
type ownerTestEnv struct {
	server      *httptest.Server
	superKey    string
	platformKey string
	productAKey string // product team "alpha"
	productBKey string // product team "beta"
}

func setupOwnershipTestServer(t *testing.T) *ownerTestEnv {
	t.Helper()

	if testPool == nil {
		t.Skip("skipping: test database not available")
	}

	ctx := context.Background()

	// Truncate for clean slate
	_, err := testPool.Exec(ctx, "TRUNCATE TABLE databases CASCADE")
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, "TRUNCATE TABLE users CASCADE")
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, "TRUNCATE TABLE teams CASCADE")
	require.NoError(t, err)

	repo := database.NewRepository(testPool)
	mgr := &dbMockManager{}
	teamRepo := team.NewRepository(testPool)
	userRepo := auth.NewRepository(testPool)
	authService := auth.NewService(userRepo, teamRepo, 4)

	// Bootstrap superuser
	superKey, err := authService.BootstrapSuperuser(ctx)
	require.NoError(t, err)

	// Create platform team + user
	platformTeam := &team.Team{Name: "platform-ops", Role: "platform"}
	require.NoError(t, teamRepo.Create(ctx, platformTeam))
	platformKey := createUserWithKey(t, authService, userRepo, "platform-user", &platformTeam.ID)

	// Create product team "alpha" + user
	alphaTeam := &team.Team{Name: "alpha", Role: "product"}
	require.NoError(t, teamRepo.Create(ctx, alphaTeam))
	productAKey := createUserWithKey(t, authService, userRepo, "alpha-user", &alphaTeam.ID)

	// Create product team "beta" + user
	betaTeam := &team.Team{Name: "beta", Role: "product"}
	require.NoError(t, teamRepo.Create(ctx, betaTeam))
	productBKey := createUserWithKey(t, authService, userRepo, "beta-user", &betaTeam.ID)

	// Create additional teams used in tests
	gammaTeam := &team.Team{Name: "gamma", Role: "product"}
	require.NoError(t, teamRepo.Create(ctx, gammaTeam))
	_ = gammaTeam

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
		UserRepo:    userRepo,
	})

	server := httptest.NewServer(router)
	t.Cleanup(func() { server.Close() })

	return &ownerTestEnv{
		server:      server,
		superKey:    superKey,
		platformKey: platformKey,
		productAKey: productAKey,
		productBKey: productBKey,
	}
}

func createUserWithKey(t *testing.T, svc *auth.Service, repo auth.UserRepository, name string, teamID *uuid.UUID) string {
	t.Helper()
	rawKey, prefix, hash, err := svc.GenerateKey()
	require.NoError(t, err)
	u := &auth.User{
		Name:         name,
		TeamID:       teamID,
		IsSuperuser:  false,
		ApiKeyPrefix: prefix,
		ApiKeyHash:   hash,
	}
	require.NoError(t, repo.Create(context.Background(), u))
	return rawKey
}

// ===== Product User Ownership Scoping =====

func TestProductUser_CreateDB_AutoSetsOwnerTeam(t *testing.T) {
	env := setupOwnershipTestServer(t)

	// Product user creates DB without specifying ownerTeam -> auto-set to team name
	body := map[string]interface{}{
		"name":    "alpha-db-auto",
		"purpose": "test auto owner",
	}
	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body, env.productAKey)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	data := result["data"].(map[string]interface{})
	assert.Equal(t, "alpha", data["ownerTeam"])
}

func TestProductUser_CreateDB_ExplicitOwnTeam(t *testing.T) {
	env := setupOwnershipTestServer(t)

	// Product user creates DB with their own team name -> allowed
	body := map[string]interface{}{
		"name":      "alpha-db-explicit",
		"ownerTeam": "alpha",
		"purpose":   "test explicit owner",
	}
	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body, env.productAKey)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	data := result["data"].(map[string]interface{})
	assert.Equal(t, "alpha", data["ownerTeam"])
}

func TestProductUser_CreateDB_OtherTeam_Forbidden(t *testing.T) {
	env := setupOwnershipTestServer(t)

	// Product user tries to create DB for another team -> 403
	body := map[string]interface{}{
		"name":      "alpha-db-sneaky",
		"ownerTeam": "beta",
		"purpose":   "trying to create for another team",
	}
	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body, env.productAKey)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "FORBIDDEN", errObj["code"])
}

func TestProductUser_ListDB_SeesOnlyOwn(t *testing.T) {
	env := setupOwnershipTestServer(t)

	// Platform user creates DBs for alpha and beta
	for _, name := range []string{"list-alpha-db", "list-beta-db"} {
		ownerTeam := "alpha"
		if name == "list-beta-db" {
			ownerTeam = "beta"
		}
		body := map[string]interface{}{
			"name":      name,
			"ownerTeam": ownerTeam,
		}
		resp, _ := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body, env.platformKey)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	}

	// Alpha user lists -> sees only alpha's DB
	resp, result := dbDoRequest(t, http.MethodGet, env.server.URL+"/databases", nil, env.productAKey)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	listData := result["data"].([]interface{})
	assert.Len(t, listData, 1)
	assert.Equal(t, "alpha", listData[0].(map[string]interface{})["ownerTeam"])

	// Beta user lists -> sees only beta's DB
	resp, result = dbDoRequest(t, http.MethodGet, env.server.URL+"/databases", nil, env.productBKey)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	listData = result["data"].([]interface{})
	assert.Len(t, listData, 1)
	assert.Equal(t, "beta", listData[0].(map[string]interface{})["ownerTeam"])

	// Platform user lists -> sees all
	resp, result = dbDoRequest(t, http.MethodGet, env.server.URL+"/databases", nil, env.platformKey)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	listData = result["data"].([]interface{})
	assert.Len(t, listData, 2)
}

func TestProductUser_GetByID_CrossTeam_NotFound(t *testing.T) {
	env := setupOwnershipTestServer(t)

	// Platform creates a DB owned by beta
	body := map[string]interface{}{
		"name":      "beta-only-db",
		"ownerTeam": "beta",
	}
	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body, env.platformKey)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	dbID := result["data"].(map[string]interface{})["id"].(string)

	// Alpha tries to get beta's DB -> 404 (not 403, to avoid leaking info)
	resp, result = dbDoRequest(t, http.MethodGet, env.server.URL+"/databases/"+dbID, nil, env.productAKey)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])

	// Beta can get it
	resp, result = dbDoRequest(t, http.MethodGet, env.server.URL+"/databases/"+dbID, nil, env.productBKey)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "beta-only-db", result["data"].(map[string]interface{})["name"])
}

func TestProductUser_Delete_CrossTeam_NotFound(t *testing.T) {
	env := setupOwnershipTestServer(t)

	// Platform creates a DB owned by alpha
	body := map[string]interface{}{
		"name":      "alpha-delete-test",
		"ownerTeam": "alpha",
	}
	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body, env.platformKey)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	dbID := result["data"].(map[string]interface{})["id"].(string)

	// Beta tries to delete alpha's DB -> 404
	resp, _ = dbDoRequest(t, http.MethodDelete, env.server.URL+"/databases/"+dbID, nil, env.productBKey)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)

	// Alpha can delete it
	resp, _ = dbDoRequest(t, http.MethodDelete, env.server.URL+"/databases/"+dbID, nil, env.productAKey)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestProductUser_Update_CannotChangeOwnerTeam(t *testing.T) {
	env := setupOwnershipTestServer(t)

	// Alpha creates a DB
	body := map[string]interface{}{
		"name":    "alpha-update-test",
		"purpose": "original",
	}
	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body, env.productAKey)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	dbID := result["data"].(map[string]interface{})["id"].(string)

	// Alpha tries to change ownerTeam -> 403
	updateBody := map[string]interface{}{
		"ownerTeam": "beta",
	}
	resp, result = dbDoRequest(t, http.MethodPatch, env.server.URL+"/databases/"+dbID, updateBody, env.productAKey)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "FORBIDDEN", errObj["code"])

	// Alpha can update purpose
	updateBody2 := map[string]interface{}{
		"purpose": "updated purpose",
	}
	resp, result = dbDoRequest(t, http.MethodPatch, env.server.URL+"/databases/"+dbID, updateBody2, env.productAKey)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	data := result["data"].(map[string]interface{})
	assert.Equal(t, "updated purpose", data["purpose"])
}

func TestProductUser_Update_CrossTeam_NotFound(t *testing.T) {
	env := setupOwnershipTestServer(t)

	// Platform creates a DB owned by beta
	body := map[string]interface{}{
		"name":      "beta-update-test",
		"ownerTeam": "beta",
	}
	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body, env.platformKey)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	dbID := result["data"].(map[string]interface{})["id"].(string)

	// Alpha tries to update beta's DB -> 404
	updateBody := map[string]interface{}{
		"purpose": "hijacked",
	}
	resp, result = dbDoRequest(t, http.MethodPatch, env.server.URL+"/databases/"+dbID, updateBody, env.productAKey)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

// ===== Platform User Full Access =====

func TestPlatformUser_SeesAllDatabases(t *testing.T) {
	env := setupOwnershipTestServer(t)

	// Create DBs with different owners
	for _, info := range []struct{ name, owner string }{
		{"plat-test-a", "alpha"},
		{"plat-test-b", "beta"},
		{"plat-test-c", "gamma"},
	} {
		body := map[string]interface{}{
			"name":      info.name,
			"ownerTeam": info.owner,
		}
		resp, _ := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body, env.platformKey)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	}

	// Platform lists all
	resp, result := dbDoRequest(t, http.MethodGet, env.server.URL+"/databases", nil, env.platformKey)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	listData := result["data"].([]interface{})
	assert.Len(t, listData, 3)
}

func TestPlatformUser_CanFilterByTeam(t *testing.T) {
	env := setupOwnershipTestServer(t)

	// Create DBs
	for _, info := range []struct{ name, owner string }{
		{"filter-a1", "alpha"},
		{"filter-a2", "alpha"},
		{"filter-b1", "beta"},
	} {
		body := map[string]interface{}{
			"name":      info.name,
			"ownerTeam": info.owner,
		}
		resp, _ := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body, env.platformKey)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	}

	// Platform filters by alpha
	resp, result := dbDoRequest(t, http.MethodGet, env.server.URL+"/databases?owner_team=alpha", nil, env.platformKey)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	listData := result["data"].([]interface{})
	assert.Len(t, listData, 2)
}

func TestPlatformUser_CanChangeOwnerTeam(t *testing.T) {
	env := setupOwnershipTestServer(t)

	// Platform creates a DB owned by alpha
	body := map[string]interface{}{
		"name":      "transfer-db",
		"ownerTeam": "alpha",
	}
	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body, env.platformKey)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	dbID := result["data"].(map[string]interface{})["id"].(string)

	// Platform changes ownerTeam to beta
	updateBody := map[string]interface{}{
		"ownerTeam": "beta",
	}
	resp, result = dbDoRequest(t, http.MethodPatch, env.server.URL+"/databases/"+dbID, updateBody, env.platformKey)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	data := result["data"].(map[string]interface{})
	assert.Equal(t, "beta", data["ownerTeam"])
}
