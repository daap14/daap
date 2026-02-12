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
	"github.com/daap14/daap/internal/blueprint"
	"github.com/daap14/daap/internal/database"
	"github.com/daap14/daap/internal/k8s"
	"github.com/daap14/daap/internal/provider"
	"github.com/daap14/daap/internal/team"
	"github.com/daap14/daap/internal/tier"
)

// blueprintTestEnv holds the test server and keys for blueprint integration tests.
type blueprintTestEnv struct {
	server      *httptest.Server
	superKey    string
	platformKey string
	productKey  string
}

func setupBlueprintTestServer(t *testing.T) *blueprintTestEnv {
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
	_, err = testPool.Exec(ctx, "TRUNCATE TABLE blueprints CASCADE")
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, "TRUNCATE TABLE users CASCADE")
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, "TRUNCATE TABLE teams CASCADE")
	require.NoError(t, err)

	repo := database.NewRepository(testPool)
	teamRepo := team.NewRepository(testPool)
	tierRepo := tier.NewPostgresRepository(testPool)
	bpRepo := blueprint.NewPostgresRepository(testPool)
	userRepo := auth.NewRepository(testPool)
	authService := auth.NewService(userRepo, teamRepo, 4)

	registry := provider.NewRegistry()
	registry.Register("cnpg", &mockProvider{})

	// Bootstrap superuser
	superKey, err := authService.BootstrapSuperuser(ctx)
	require.NoError(t, err)

	// Create platform team + user
	platformTeam := &team.Team{Name: "platform-ops", Role: "platform"}
	require.NoError(t, teamRepo.Create(ctx, platformTeam))
	platformKey := createUserWithKey(t, authService, userRepo, "platform-bp-user", &platformTeam.ID)

	// Create product team + user
	productTeam := &team.Team{Name: "product-team", Role: "product"}
	require.NoError(t, teamRepo.Create(ctx, productTeam))
	productKey := createUserWithKey(t, authService, userRepo, "product-bp-user", &productTeam.ID)

	checker := &mockHealthChecker{
		status: k8s.ConnectivityStatus{Connected: true, Version: "v1.31.0"},
	}
	pinger := &dbTestPinger{pool: testPool}

	router := api.NewRouter(api.RouterDeps{
		K8sChecker:       checker,
		DBPinger:         pinger,
		Version:          "0.1.0-test",
		Repo:             repo,
		Namespace:        "default",
		AuthService:      authService,
		TeamRepo:         teamRepo,
		TierRepo:         tierRepo,
		BlueprintRepo:    bpRepo,
		ProviderRegistry: registry,
		UserRepo:         userRepo,
	})

	server := httptest.NewServer(router)
	t.Cleanup(func() { server.Close() })

	return &blueprintTestEnv{
		server:      server,
		superKey:    superKey,
		platformKey: platformKey,
		productKey:  productKey,
	}
}

// ===== Blueprint Lifecycle Test =====

func TestBlueprintLifecycle(t *testing.T) {
	env := setupBlueprintTestServer(t)

	var bpID string

	// Step 1: Platform user creates a blueprint
	t.Run("create blueprint", func(t *testing.T) {
		body := map[string]interface{}{
			"name":      "cnpg-standard",
			"provider":  "cnpg",
			"manifests": testManifest,
		}
		resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/blueprints", body, env.platformKey)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		assert.Nil(t, result["error"])

		data := result["data"].(map[string]interface{})
		bpID = data["id"].(string)
		assert.NotEmpty(t, bpID)
		assert.Equal(t, "cnpg-standard", data["name"])
		assert.Equal(t, "cnpg", data["provider"])
		assert.Equal(t, testManifest, data["manifests"])
		assert.NotEmpty(t, data["createdAt"])
		assert.NotEmpty(t, data["updatedAt"])
	})

	// Step 2: List blueprints
	t.Run("list blueprints", func(t *testing.T) {
		resp, result := dbDoRequest(t, http.MethodGet, env.server.URL+"/blueprints", nil, env.platformKey)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		listData := result["data"].([]interface{})
		require.Len(t, listData, 1)

		item := listData[0].(map[string]interface{})
		assert.Equal(t, bpID, item["id"])
		assert.Equal(t, "cnpg-standard", item["name"])
		assert.Equal(t, "cnpg", item["provider"])
	})

	// Step 3: Get blueprint by ID
	t.Run("get blueprint by ID", func(t *testing.T) {
		resp, result := dbDoRequest(t, http.MethodGet, env.server.URL+"/blueprints/"+bpID, nil, env.platformKey)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		data := result["data"].(map[string]interface{})
		assert.Equal(t, bpID, data["id"])
		assert.Equal(t, "cnpg-standard", data["name"])
		assert.Equal(t, "cnpg", data["provider"])
		assert.Equal(t, testManifest, data["manifests"])
	})

	// Step 4: Delete blueprint
	t.Run("delete blueprint", func(t *testing.T) {
		resp, _ := dbDoRequest(t, http.MethodDelete, env.server.URL+"/blueprints/"+bpID, nil, env.platformKey)
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	// Step 5: Confirm deleted -> 404
	t.Run("blueprint not found after delete", func(t *testing.T) {
		resp, result := dbDoRequest(t, http.MethodGet, env.server.URL+"/blueprints/"+bpID, nil, env.platformKey)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		errObj := result["error"].(map[string]interface{})
		assert.Equal(t, "NOT_FOUND", errObj["code"])
	})
}

// ===== Duplicate Name =====

func TestBlueprintCreate_DuplicateName(t *testing.T) {
	env := setupBlueprintTestServer(t)

	body := map[string]interface{}{
		"name":      "dup-bp",
		"provider":  "cnpg",
		"manifests": testManifest,
	}

	resp, _ := dbDoRequest(t, http.MethodPost, env.server.URL+"/blueprints", body, env.platformKey)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/blueprints", body, env.platformKey)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "DUPLICATE_NAME", errObj["code"])
}

// ===== Cannot Delete Blueprint With Active Tiers =====

func TestBlueprintDelete_HasTiers(t *testing.T) {
	env := setupBlueprintTestServer(t)

	// Create blueprint
	bpBody := map[string]interface{}{
		"name":      "bp-with-tiers",
		"provider":  "cnpg",
		"manifests": testManifest,
	}
	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/blueprints", bpBody, env.platformKey)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	bpID := result["data"].(map[string]interface{})["id"].(string)

	// Create tier referencing this blueprint
	tierBody := map[string]interface{}{
		"name":                "ref-tier",
		"blueprintName":       "bp-with-tiers",
		"destructionStrategy": "hard_delete",
	}
	resp, _ = dbDoRequest(t, http.MethodPost, env.server.URL+"/tiers", tierBody, env.platformKey)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// Try to delete blueprint -> 409
	resp, result = dbDoRequest(t, http.MethodDelete, env.server.URL+"/blueprints/"+bpID, nil, env.platformKey)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "BLUEPRINT_HAS_TIERS", errObj["code"])
}

// ===== Invalid Provider =====

func TestBlueprintCreate_InvalidProvider(t *testing.T) {
	env := setupBlueprintTestServer(t)

	body := map[string]interface{}{
		"name":      "bad-provider-bp",
		"provider":  "nonexistent-provider",
		"manifests": testManifest,
	}
	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/blueprints", body, env.platformKey)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])
}

// ===== Product User Can Read Blueprints =====

func TestProductUser_CanReadBlueprints(t *testing.T) {
	env := setupBlueprintTestServer(t)

	// Platform creates a blueprint
	bpBody := map[string]interface{}{
		"name":      "readable-bp",
		"provider":  "cnpg",
		"manifests": testManifest,
	}
	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/blueprints", bpBody, env.platformKey)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	bpID := result["data"].(map[string]interface{})["id"].(string)

	// Product user can list blueprints
	resp, result = dbDoRequest(t, http.MethodGet, env.server.URL+"/blueprints", nil, env.productKey)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	listData := result["data"].([]interface{})
	assert.Len(t, listData, 1)

	// Product user can get blueprint by ID
	resp, result = dbDoRequest(t, http.MethodGet, env.server.URL+"/blueprints/"+bpID, nil, env.productKey)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	data := result["data"].(map[string]interface{})
	assert.Equal(t, "readable-bp", data["name"])
}

// ===== Product User Cannot Manage Blueprints =====

func TestProductUser_CannotManageBlueprints(t *testing.T) {
	env := setupBlueprintTestServer(t)

	// Product user tries to create -> 403
	body := map[string]interface{}{
		"name":      "sneaky-bp",
		"provider":  "cnpg",
		"manifests": testManifest,
	}
	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/blueprints", body, env.productKey)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "FORBIDDEN", errObj["code"])

	// Platform creates a blueprint
	resp, result = dbDoRequest(t, http.MethodPost, env.server.URL+"/blueprints", body, env.platformKey)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	bpID := result["data"].(map[string]interface{})["id"].(string)

	// Product user tries to delete -> 403
	resp, result = dbDoRequest(t, http.MethodDelete, env.server.URL+"/blueprints/"+bpID, nil, env.productKey)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	errObj = result["error"].(map[string]interface{})
	assert.Equal(t, "FORBIDDEN", errObj["code"])
}

// ===== Superuser Cannot Access Blueprint Endpoints =====

func TestSuperuser_CannotAccessBlueprints(t *testing.T) {
	env := setupBlueprintTestServer(t)

	// Superuser tries to list blueprints -> 403 (RequireRole rejects nil role)
	resp, result := dbDoRequest(t, http.MethodGet, env.server.URL+"/blueprints", nil, env.superKey)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "FORBIDDEN", errObj["code"])

	// Superuser tries to create blueprint -> 403
	body := map[string]interface{}{
		"name":      "super-bp",
		"provider":  "cnpg",
		"manifests": testManifest,
	}
	resp, result = dbDoRequest(t, http.MethodPost, env.server.URL+"/blueprints", body, env.superKey)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	errObj = result["error"].(map[string]interface{})
	assert.Equal(t, "FORBIDDEN", errObj["code"])
}
