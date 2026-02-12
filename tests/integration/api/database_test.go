package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
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

const defaultDBTestURL = "postgres://daap:daap@127.0.0.1:5433/daap_test?sslmode=disable"

var testPool *pgxpool.Pool

const createBlueprintsTableSQL = `
CREATE TABLE IF NOT EXISTS blueprints (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(63) NOT NULL UNIQUE,
    provider VARCHAR(63) NOT NULL,
    manifests TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_blueprints_name ON blueprints (name);
CREATE INDEX IF NOT EXISTS idx_blueprints_provider ON blueprints (provider);
`

const createTiersTableSQL = `
CREATE TABLE IF NOT EXISTS tiers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(63) NOT NULL UNIQUE,
    description TEXT NOT NULL DEFAULT '',
    blueprint_id UUID REFERENCES blueprints(id) ON DELETE RESTRICT,
    destruction_strategy VARCHAR(20) NOT NULL DEFAULT 'hard_delete'
        CHECK (destruction_strategy IN ('freeze', 'archive', 'hard_delete')),
    backup_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_tiers_name ON tiers (name);
`

const createTableSQL = `
CREATE TABLE IF NOT EXISTS databases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(63) NOT NULL,
    owner_team_id UUID NOT NULL,
    tier_id UUID REFERENCES tiers(id) ON DELETE RESTRICT,
    purpose TEXT NOT NULL DEFAULT '',
    namespace VARCHAR(255) NOT NULL DEFAULT 'default',
    cluster_name VARCHAR(255) NOT NULL,
    pooler_name VARCHAR(255) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'provisioning',
    host VARCHAR(255),
    port INTEGER,
    secret_name VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT fk_databases_owner_team FOREIGN KEY (owner_team_id) REFERENCES teams(id) ON DELETE RESTRICT
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_databases_name_active ON databases (name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_databases_owner_team_id ON databases (owner_team_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_databases_status ON databases (status);
CREATE INDEX IF NOT EXISTS idx_databases_tier_id ON databases (tier_id);
`

const createAuthTablesSQL = `
CREATE TABLE IF NOT EXISTS teams (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    role VARCHAR(20) NOT NULL CHECK (role IN ('platform', 'product')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    team_id UUID REFERENCES teams(id) ON DELETE RESTRICT,
    is_superuser BOOLEAN NOT NULL DEFAULT FALSE,
    api_key_prefix VARCHAR(8) NOT NULL,
    api_key_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_users_superuser ON users (is_superuser) WHERE is_superuser = TRUE;
CREATE INDEX IF NOT EXISTS idx_users_api_key_prefix ON users (api_key_prefix) WHERE revoked_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_users_team_id ON users (team_id);
`

// --- Mock provider for integration tests ---

type mockProvider struct{}

func (m *mockProvider) Apply(_ context.Context, _ provider.ProviderDatabase, _ string) error {
	return nil
}

func (m *mockProvider) Delete(_ context.Context, _ provider.ProviderDatabase) error {
	return nil
}

func (m *mockProvider) CheckHealth(_ context.Context, _ provider.ProviderDatabase) (provider.HealthResult, error) {
	return provider.HealthResult{Status: "provisioning"}, nil
}

const testManifest = `---
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: daap-{{ .Name }}
  namespace: {{ .Namespace }}
spec:
  instances: 1
`

func TestMain(m *testing.M) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		dbURL = defaultDBTestURL
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Printf("Skipping database integration tests: cannot connect: %v", err)
		os.Exit(0)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		log.Printf("Skipping database integration tests: cannot ping: %v", err)
		os.Exit(0)
	}

	// Run migrations (order matters due to FK constraints: auth -> blueprints -> tiers -> databases)
	if _, err := pool.Exec(ctx, createAuthTablesSQL); err != nil {
		pool.Close()
		log.Fatalf("Failed to run auth migration: %v", err)
	}
	if _, err := pool.Exec(ctx, createBlueprintsTableSQL); err != nil {
		pool.Close()
		log.Fatalf("Failed to run blueprints migration: %v", err)
	}
	if _, err := pool.Exec(ctx, createTiersTableSQL); err != nil {
		pool.Close()
		log.Fatalf("Failed to run tiers migration: %v", err)
	}
	if _, err := pool.Exec(ctx, createTableSQL); err != nil {
		pool.Close()
		log.Fatalf("Failed to run database migration: %v", err)
	}

	testPool = pool
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// --- Mock DBPinger that uses the real pool ---

type dbTestPinger struct{ pool *pgxpool.Pool }

func (p *dbTestPinger) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }

// --- Test server setup ---

type dbTestEnv struct {
	server *httptest.Server
	apiKey string
}

func setupDBTestServer(t *testing.T) *dbTestEnv {
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
	authService := auth.NewService(userRepo, teamRepo, 4) // low cost for test speed

	registry := provider.NewRegistry()
	registry.Register("cnpg", &mockProvider{})

	// Create a default blueprint for tests
	defaultBP := &blueprint.Blueprint{
		Name:      "cnpg-default",
		Provider:  "cnpg",
		Manifests: testManifest,
	}
	err = bpRepo.Create(ctx, defaultBP)
	require.NoError(t, err)

	// Create a default tier for database tests
	defaultTier := &tier.Tier{
		Name:                "standard",
		Description:         "Standard tier for tests",
		BlueprintID:         &defaultBP.ID,
		DestructionStrategy: "hard_delete",
		BackupEnabled:       false,
	}
	err = tierRepo.Create(ctx, defaultTier)
	require.NoError(t, err)

	// Create a platform team and a platform user for authenticated requests
	platformTeam := &team.Team{Name: "test-platform", Role: "platform"}
	err = teamRepo.Create(ctx, platformTeam)
	require.NoError(t, err)

	// Create teams used by database integration tests
	for _, tc := range []struct{ name, role string }{
		{"platform", "platform"},
		{"backend", "product"},
		{"alpha", "product"},
		{"beta", "product"},
	} {
		tm := &team.Team{Name: tc.name, Role: tc.role}
		err = teamRepo.Create(ctx, tm)
		require.NoError(t, err)
	}

	rawKey, prefix, hash, err := authService.GenerateKey()
	require.NoError(t, err)

	platformUser := &auth.User{
		Name:         "test-platform-user",
		TeamID:       &platformTeam.ID,
		IsSuperuser:  false,
		ApiKeyPrefix: prefix,
		ApiKeyHash:   hash,
	}
	err = userRepo.Create(ctx, platformUser)
	require.NoError(t, err)

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

	return &dbTestEnv{server: server, apiKey: rawKey}
}

// --- HTTP helper ---

func dbDoRequest(t *testing.T, method, url string, body interface{}, apiKey string) (*http.Response, map[string]interface{}) {
	t.Helper()

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, url, reqBody)
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if apiKey != "" {
		req.Header.Set("X-API-Key", apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	resp.Body.Close()

	if len(respBody) == 0 {
		return resp, nil
	}

	var env map[string]interface{}
	err = json.Unmarshal(respBody, &env)
	require.NoError(t, err, "failed to parse response: %s", string(respBody))

	return resp, env
}

// ===== Full Lifecycle Test =====

func TestDatabaseLifecycle(t *testing.T) {
	env := setupDBTestServer(t)

	// Step 1: POST /databases -> 201 with id and status="provisioning"
	t.Run("Create", func(t *testing.T) {})

	createBody := map[string]interface{}{
		"name":      "lifecycle-db",
		"ownerTeam": "platform",
		"tier":      "standard",
		"purpose":   "integration test",
	}

	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", createBody, env.apiKey)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Nil(t, result["error"])
	assert.NotNil(t, result["meta"])

	data := result["data"].(map[string]interface{})
	dbID := data["id"].(string)
	assert.NotEmpty(t, dbID)
	assert.Equal(t, "lifecycle-db", data["name"])
	assert.Equal(t, "platform", data["ownerTeam"])
	assert.Equal(t, "integration test", data["purpose"])
	assert.Equal(t, "provisioning", data["status"])
	assert.Equal(t, "daap-lifecycle-db", data["clusterName"])
	assert.Equal(t, "daap-lifecycle-db-pooler", data["poolerName"])
	assert.NotEmpty(t, data["createdAt"])
	assert.NotEmpty(t, data["updatedAt"])

	// Verify envelope structure
	assert.Contains(t, result, "data")
	assert.Contains(t, result, "error")
	assert.Contains(t, result, "meta")
	meta := result["meta"].(map[string]interface{})
	assert.NotEmpty(t, meta["requestId"])
	assert.NotEmpty(t, meta["timestamp"])

	// Verify X-Request-ID header
	assert.NotEmpty(t, resp.Header.Get("X-Request-ID"))

	// Step 2: GET /databases -> list includes the new database
	resp, result = dbDoRequest(t, http.MethodGet, env.server.URL+"/databases", nil, env.apiKey)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	listData := result["data"].([]interface{})
	assert.Len(t, listData, 1)

	listMeta := result["meta"].(map[string]interface{})
	assert.Equal(t, float64(1), listMeta["total"])
	assert.Equal(t, float64(1), listMeta["page"])
	assert.Equal(t, float64(20), listMeta["limit"])

	firstItem := listData[0].(map[string]interface{})
	assert.Equal(t, dbID, firstItem["id"])
	assert.Equal(t, "provisioning", firstItem["status"])
	assert.Nil(t, firstItem["host"])
	assert.Nil(t, firstItem["port"])

	// Step 3: GET /databases/:id -> returns the database
	resp, result = dbDoRequest(t, http.MethodGet, env.server.URL+"/databases/"+dbID, nil, env.apiKey)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	data = result["data"].(map[string]interface{})
	assert.Equal(t, dbID, data["id"])
	assert.Equal(t, "lifecycle-db", data["name"])
	assert.Equal(t, "platform", data["ownerTeam"])
	assert.Equal(t, "integration test", data["purpose"])

	// Step 4: PATCH /databases/:id -> 200, purpose updated
	updateBody := map[string]interface{}{
		"ownerTeam": "backend",
		"purpose":   "updated purpose",
	}
	resp, result = dbDoRequest(t, http.MethodPatch, env.server.URL+"/databases/"+dbID, updateBody, env.apiKey)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	data = result["data"].(map[string]interface{})
	assert.Equal(t, "backend", data["ownerTeam"])
	assert.Equal(t, "updated purpose", data["purpose"])
	assert.Equal(t, "lifecycle-db", data["name"])

	// Verify update persisted via GET
	resp, result = dbDoRequest(t, http.MethodGet, env.server.URL+"/databases/"+dbID, nil, env.apiKey)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	data = result["data"].(map[string]interface{})
	assert.Equal(t, "backend", data["ownerTeam"])
	assert.Equal(t, "updated purpose", data["purpose"])

	// Step 5: DELETE /databases/:id -> 204
	resp, _ = dbDoRequest(t, http.MethodDelete, env.server.URL+"/databases/"+dbID, nil, env.apiKey)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	// Step 6: GET /databases/:id -> 404 (soft-deleted)
	resp, result = dbDoRequest(t, http.MethodGet, env.server.URL+"/databases/"+dbID, nil, env.apiKey)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])

	// Step 7: GET /databases -> empty list
	resp, result = dbDoRequest(t, http.MethodGet, env.server.URL+"/databases", nil, env.apiKey)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	listData = result["data"].([]interface{})
	assert.Len(t, listData, 0)
	listMeta = result["meta"].(map[string]interface{})
	assert.Equal(t, float64(0), listMeta["total"])
}

// ===== Validation Errors =====

func TestCreate_InvalidName(t *testing.T) {
	env := setupDBTestServer(t)

	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", map[string]interface{}{
		"name":      "INVALID_NAME",
		"ownerTeam": "platform",
		"tier":      "standard",
	}, env.apiKey)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])
	assert.NotNil(t, errObj["details"])
}

func TestCreate_MissingRequired(t *testing.T) {
	env := setupDBTestServer(t)

	// Missing ownerTeam
	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", map[string]interface{}{
		"name":      "valid-name",
		"ownerTeam": "",
		"tier":      "standard",
	}, env.apiKey)
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])
	assert.NotNil(t, errObj["details"])

	details := errObj["details"].([]interface{})
	hasOwnerTeamErr := false
	for _, d := range details {
		field := d.(map[string]interface{})
		if field["field"] == "ownerTeam" {
			hasOwnerTeamErr = true
		}
	}
	assert.True(t, hasOwnerTeamErr, "expected ownerTeam validation error")
}

// ===== Pagination =====

func TestList_PaginationIntegration(t *testing.T) {
	env := setupDBTestServer(t)

	// Create 3 databases
	for i := 0; i < 3; i++ {
		body := map[string]interface{}{
			"name":      fmt.Sprintf("paginate-%d-db", i),
			"ownerTeam": "platform",
			"tier":      "standard",
		}
		resp, _ := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body, env.apiKey)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	}

	// Page 1, limit 2 -> 2 results, total=3
	resp, result := dbDoRequest(t, http.MethodGet, env.server.URL+"/databases?page=1&limit=2", nil, env.apiKey)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	listData := result["data"].([]interface{})
	assert.Len(t, listData, 2)

	meta := result["meta"].(map[string]interface{})
	assert.Equal(t, float64(3), meta["total"])
	assert.Equal(t, float64(1), meta["page"])
	assert.Equal(t, float64(2), meta["limit"])

	// Page 2, limit 2 -> 1 result
	resp, result = dbDoRequest(t, http.MethodGet, env.server.URL+"/databases?page=2&limit=2", nil, env.apiKey)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	listData = result["data"].([]interface{})
	assert.Len(t, listData, 1)
}

// ===== Filter by Team =====

func TestList_FilterByTeam(t *testing.T) {
	env := setupDBTestServer(t)

	// Create databases with different owner teams
	for i, team := range []string{"alpha", "alpha", "beta"} {
		body := map[string]interface{}{
			"name":      fmt.Sprintf("filter-%s-%d", team, i),
			"ownerTeam": team,
			"tier":      "standard",
		}
		resp, _ := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body, env.apiKey)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	}

	// Filter by alpha -> 2 results
	resp, result := dbDoRequest(t, http.MethodGet, env.server.URL+"/databases?owner_team=alpha", nil, env.apiKey)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	listData := result["data"].([]interface{})
	assert.Len(t, listData, 2)
	for _, item := range listData {
		db := item.(map[string]interface{})
		assert.Equal(t, "alpha", db["ownerTeam"])
	}

	// Filter by beta -> 1 result
	resp, result = dbDoRequest(t, http.MethodGet, env.server.URL+"/databases?owner_team=beta", nil, env.apiKey)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	listData = result["data"].([]interface{})
	assert.Len(t, listData, 1)
	assert.Equal(t, "beta", listData[0].(map[string]interface{})["ownerTeam"])

	// Filter by nonexistent -> 0 results
	resp, result = dbDoRequest(t, http.MethodGet, env.server.URL+"/databases?owner_team=gamma", nil, env.apiKey)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	listData = result["data"].([]interface{})
	assert.Len(t, listData, 0)
}

// ===== Duplicate Name =====

func TestCreate_DuplicateNameIntegration(t *testing.T) {
	env := setupDBTestServer(t)

	body := map[string]interface{}{
		"name":      "unique-db",
		"ownerTeam": "platform",
		"tier":      "standard",
	}

	resp, _ := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body, env.apiKey)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body, env.apiKey)
	assert.Equal(t, http.StatusConflict, resp.StatusCode)
	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "DUPLICATE_NAME", errObj["code"])
}

// ===== Name Reuse After Delete =====

func TestCreate_NameReuseAfterDelete(t *testing.T) {
	env := setupDBTestServer(t)

	body := map[string]interface{}{
		"name":      "reusable-db",
		"ownerTeam": "platform",
		"tier":      "standard",
	}

	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body, env.apiKey)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	dbID := result["data"].(map[string]interface{})["id"].(string)

	resp, _ = dbDoRequest(t, http.MethodDelete, env.server.URL+"/databases/"+dbID, nil, env.apiKey)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	resp, result = dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body, env.apiKey)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	newID := result["data"].(map[string]interface{})["id"].(string)
	assert.NotEqual(t, dbID, newID)
}
