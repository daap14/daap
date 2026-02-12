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

// tierTestEnv holds the test server and keys for tier integration tests.
type tierTestEnv struct {
	server      *httptest.Server
	superKey    string
	platformKey string
	productKey  string
}

func setupTierTestServer(t *testing.T) *tierTestEnv {
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

	// Create default blueprint for tier tests
	defaultBP := &blueprint.Blueprint{
		Name:      "cnpg-default",
		Provider:  "cnpg",
		Manifests: testManifest,
	}
	err = bpRepo.Create(ctx, defaultBP)
	require.NoError(t, err)

	// Bootstrap superuser
	superKey, err := authService.BootstrapSuperuser(ctx)
	require.NoError(t, err)

	// Create platform team + user
	platformTeam := &team.Team{Name: "platform-ops", Role: "platform"}
	require.NoError(t, teamRepo.Create(ctx, platformTeam))
	platformKey := createUserWithKey(t, authService, userRepo, "platform-tier-user", &platformTeam.ID)

	// Create product team + user
	productTeam := &team.Team{Name: "product-team", Role: "product"}
	require.NoError(t, teamRepo.Create(ctx, productTeam))
	productKey := createUserWithKey(t, authService, userRepo, "product-tier-user", &productTeam.ID)

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

	return &tierTestEnv{
		server:      server,
		superKey:    superKey,
		platformKey: platformKey,
		productKey:  productKey,
	}
}

// ===== Tier Lifecycle Test =====

func TestTierLifecycle(t *testing.T) {
	env := setupTierTestServer(t)

	// Step 1: Platform user creates a tier
	var tierID string
	t.Run("platform creates tier", func(t *testing.T) {
		body := map[string]interface{}{
			"name":                "standard",
			"description":         "Standard production tier",
			"blueprintName":       "cnpg-default",
			"destructionStrategy": "freeze",
			"backupEnabled":       true,
		}
		resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/tiers", body, env.platformKey)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		assert.Nil(t, result["error"])

		data := result["data"].(map[string]interface{})
		tierID = data["id"].(string)
		assert.NotEmpty(t, tierID)
		assert.Equal(t, "standard", data["name"])
		assert.Equal(t, "Standard production tier", data["description"])
		assert.Equal(t, "cnpg-default", data["blueprintName"])
		assert.NotNil(t, data["blueprintId"])
		assert.Equal(t, "freeze", data["destructionStrategy"])
		assert.Equal(t, true, data["backupEnabled"])
		assert.NotEmpty(t, data["createdAt"])
		assert.NotEmpty(t, data["updatedAt"])
	})

	// Step 2: Product user lists tiers -> redacted response
	t.Run("product user sees redacted list", func(t *testing.T) {
		resp, result := dbDoRequest(t, http.MethodGet, env.server.URL+"/tiers", nil, env.productKey)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		listData := result["data"].([]interface{})
		require.Len(t, listData, 1)

		item := listData[0].(map[string]interface{})
		assert.Equal(t, tierID, item["id"])
		assert.Equal(t, "standard", item["name"])
		assert.Equal(t, "Standard production tier", item["description"])

		// Redacted fields should NOT be present
		assert.Nil(t, item["blueprintId"])
		assert.Nil(t, item["blueprintName"])
		assert.Nil(t, item["destructionStrategy"])
		assert.Nil(t, item["backupEnabled"])
		assert.Nil(t, item["createdAt"])
		assert.Nil(t, item["updatedAt"])
	})

	// Step 3: Product user gets tier by ID -> redacted
	t.Run("product user gets tier by ID redacted", func(t *testing.T) {
		resp, result := dbDoRequest(t, http.MethodGet, env.server.URL+"/tiers/"+tierID, nil, env.productKey)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		data := result["data"].(map[string]interface{})
		assert.Equal(t, tierID, data["id"])
		assert.Equal(t, "standard", data["name"])
		assert.Equal(t, "Standard production tier", data["description"])

		// No infrastructure fields
		assert.Nil(t, data["blueprintId"])
		assert.Nil(t, data["blueprintName"])
	})

	// Step 4: Platform user lists tiers -> full response
	t.Run("platform user sees full list", func(t *testing.T) {
		resp, result := dbDoRequest(t, http.MethodGet, env.server.URL+"/tiers", nil, env.platformKey)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		listData := result["data"].([]interface{})
		require.Len(t, listData, 1)

		item := listData[0].(map[string]interface{})
		assert.Equal(t, "standard", item["name"])
		assert.NotNil(t, item["blueprintId"])
		assert.Equal(t, "cnpg-default", item["blueprintName"])
		assert.Equal(t, "freeze", item["destructionStrategy"])
		assert.Equal(t, true, item["backupEnabled"])
		assert.NotNil(t, item["createdAt"])
	})

	// Step 5: Platform user updates tier
	t.Run("platform updates tier", func(t *testing.T) {
		body := map[string]interface{}{
			"description":   "Updated standard tier",
			"backupEnabled": false,
		}
		resp, result := dbDoRequest(t, http.MethodPatch, env.server.URL+"/tiers/"+tierID, body, env.platformKey)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		data := result["data"].(map[string]interface{})
		assert.Equal(t, "Updated standard tier", data["description"])
		assert.Equal(t, false, data["backupEnabled"])
		assert.Equal(t, "standard", data["name"]) // name unchanged
	})

	// Step 6: Create a database using this tier
	t.Run("create database with tier", func(t *testing.T) {
		body := map[string]interface{}{
			"name":      "tier-test-db",
			"ownerTeam": "platform-ops",
			"tier":      "standard",
			"purpose":   "testing tier integration",
		}
		resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body, env.platformKey)
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		data := result["data"].(map[string]interface{})
		assert.Equal(t, "standard", data["tier"])
		assert.Equal(t, "provisioning", data["status"])
	})

	// Step 7: Cannot delete tier with active database
	t.Run("cannot delete tier with active database", func(t *testing.T) {
		resp, result := dbDoRequest(t, http.MethodDelete, env.server.URL+"/tiers/"+tierID, nil, env.platformKey)
		assert.Equal(t, http.StatusConflict, resp.StatusCode)
		errObj := result["error"].(map[string]interface{})
		assert.Equal(t, "TIER_HAS_DATABASES", errObj["code"])
	})

	// Step 8: Delete a tier that has no databases at all
	t.Run("delete tier with no databases", func(t *testing.T) {
		body := map[string]interface{}{
			"name":                "empty-tier",
			"blueprintName":       "cnpg-default",
			"destructionStrategy": "hard_delete",
		}
		resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/tiers", body, env.platformKey)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		emptyTierID := result["data"].(map[string]interface{})["id"].(string)

		resp, _ = dbDoRequest(t, http.MethodDelete, env.server.URL+"/tiers/"+emptyTierID, nil, env.platformKey)
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)

		// Confirm it's gone
		resp, result = dbDoRequest(t, http.MethodGet, env.server.URL+"/tiers/"+emptyTierID, nil, env.platformKey)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		errObj := result["error"].(map[string]interface{})
		assert.Equal(t, "NOT_FOUND", errObj["code"])
	})
}

// ===== Product User Cannot Manage Tiers =====

func TestProductUser_CannotCreateTier(t *testing.T) {
	env := setupTierTestServer(t)

	body := map[string]interface{}{
		"name":                "sneaky-tier",
		"blueprintName":       "cnpg-default",
		"destructionStrategy": "hard_delete",
	}
	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/tiers", body, env.productKey)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "FORBIDDEN", errObj["code"])
}

func TestProductUser_CannotUpdateTier(t *testing.T) {
	env := setupTierTestServer(t)

	// Platform creates a tier first
	createBody := map[string]interface{}{
		"name":                "prod-readonly",
		"blueprintName":       "cnpg-default",
		"destructionStrategy": "hard_delete",
	}
	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/tiers", createBody, env.platformKey)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	tierID := result["data"].(map[string]interface{})["id"].(string)

	// Product user tries to update -> 403
	updateBody := map[string]interface{}{
		"description": "hacked",
	}
	resp, result = dbDoRequest(t, http.MethodPatch, env.server.URL+"/tiers/"+tierID, updateBody, env.productKey)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "FORBIDDEN", errObj["code"])
}

func TestProductUser_CannotDeleteTier(t *testing.T) {
	env := setupTierTestServer(t)

	// Platform creates a tier
	createBody := map[string]interface{}{
		"name":                "no-delete",
		"blueprintName":       "cnpg-default",
		"destructionStrategy": "hard_delete",
	}
	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/tiers", createBody, env.platformKey)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	tierID := result["data"].(map[string]interface{})["id"].(string)

	// Product user tries to delete -> 403
	resp, result = dbDoRequest(t, http.MethodDelete, env.server.URL+"/tiers/"+tierID, nil, env.productKey)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "FORBIDDEN", errObj["code"])
}

// ===== Superuser Cannot Access Tier Endpoints =====

func TestSuperuser_CannotAccessTiers(t *testing.T) {
	env := setupTierTestServer(t)

	// Superuser tries to list tiers -> 403 (RequireRole rejects nil role)
	resp, result := dbDoRequest(t, http.MethodGet, env.server.URL+"/tiers", nil, env.superKey)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "FORBIDDEN", errObj["code"])

	// Superuser tries to create tier -> 403
	body := map[string]interface{}{
		"name":                "super-tier",
		"blueprintName":       "cnpg-default",
		"destructionStrategy": "hard_delete",
	}
	resp, result = dbDoRequest(t, http.MethodPost, env.server.URL+"/tiers", body, env.superKey)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	errObj = result["error"].(map[string]interface{})
	assert.Equal(t, "FORBIDDEN", errObj["code"])
}

// ===== Tier Duplicate Name =====

func TestTierCreate_DuplicateNameIntegration(t *testing.T) {
	env := setupTierTestServer(t)

	body := map[string]interface{}{
		"name":                "unique-tier",
		"blueprintName":       "cnpg-default",
		"destructionStrategy": "hard_delete",
	}

	resp, _ := dbDoRequest(t, http.MethodPost, env.server.URL+"/tiers", body, env.platformKey)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/tiers", body, env.platformKey)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "DUPLICATE_NAME", errObj["code"])
}

// ===== Tier Update Immutable Name =====

func TestTierUpdate_ImmutableName(t *testing.T) {
	env := setupTierTestServer(t)

	createBody := map[string]interface{}{
		"name":                "immutable-name",
		"blueprintName":       "cnpg-default",
		"destructionStrategy": "hard_delete",
	}
	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/tiers", createBody, env.platformKey)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	tierID := result["data"].(map[string]interface{})["id"].(string)

	// Try to change name -> IMMUTABLE_FIELD
	updateBody := map[string]interface{}{
		"name": "new-name",
	}
	resp, result = dbDoRequest(t, http.MethodPatch, env.server.URL+"/tiers/"+tierID, updateBody, env.platformKey)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "IMMUTABLE_FIELD", errObj["code"])
}

// ===== Database Requires Valid Tier =====

func TestDatabaseCreate_InvalidTier(t *testing.T) {
	env := setupTierTestServer(t)

	body := map[string]interface{}{
		"name":      "needs-tier",
		"ownerTeam": "platform-ops",
		"tier":      "nonexistent-tier",
	}
	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body, env.platformKey)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

func TestDatabaseCreate_MissingTier(t *testing.T) {
	env := setupTierTestServer(t)

	body := map[string]interface{}{
		"name":      "no-tier-db",
		"ownerTeam": "platform-ops",
	}
	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body, env.platformKey)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])

	details := errObj["details"].([]interface{})
	hasTierErr := false
	for _, d := range details {
		field := d.(map[string]interface{})
		if field["field"] == "tier" {
			hasTierErr = true
		}
	}
	assert.True(t, hasTierErr, "expected tier validation error")
}
