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
	"github.com/daap14/daap/internal/database"
	"github.com/daap14/daap/internal/k8s"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const defaultDBTestURL = "postgres://daap:daap@127.0.0.1:5433/daap_test?sslmode=disable"

var testPool *pgxpool.Pool

const createTableSQL = `
CREATE TABLE IF NOT EXISTS databases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(63) NOT NULL,
    owner_team VARCHAR(255) NOT NULL,
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
    deleted_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_databases_name_active ON databases (name) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_databases_owner_team ON databases (owner_team);
CREATE INDEX IF NOT EXISTS idx_databases_status ON databases (status);
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

	// Run migration
	if _, err := pool.Exec(ctx, createTableSQL); err != nil {
		pool.Close()
		log.Fatalf("Failed to run migration: %v", err)
	}

	testPool = pool
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// --- Mock K8s ResourceManager for database tests ---

type dbMockManager struct {
	appliedClusters int
	appliedPoolers  int
	deletedClusters int
	deletedPoolers  int
}

func (m *dbMockManager) ApplyCluster(_ context.Context, _ *unstructured.Unstructured) error {
	m.appliedClusters++
	return nil
}

func (m *dbMockManager) ApplyPooler(_ context.Context, _ *unstructured.Unstructured) error {
	m.appliedPoolers++
	return nil
}

func (m *dbMockManager) DeleteCluster(_ context.Context, _, _ string) error {
	m.deletedClusters++
	return nil
}

func (m *dbMockManager) DeletePooler(_ context.Context, _, _ string) error {
	m.deletedPoolers++
	return nil
}

func (m *dbMockManager) GetClusterStatus(_ context.Context, _, _ string) (k8s.ClusterStatus, error) {
	return k8s.ClusterStatus{Phase: "Cluster in healthy state", Ready: true}, nil
}

func (m *dbMockManager) GetSecret(_ context.Context, _, _ string) (map[string][]byte, error) {
	return map[string][]byte{
		"username": []byte("app"),
		"password": []byte("secret"),
	}, nil
}

// --- Mock DBPinger that uses the real pool ---

type dbTestPinger struct{ pool *pgxpool.Pool }

func (p *dbTestPinger) Ping(ctx context.Context) error { return p.pool.Ping(ctx) }

// --- Test server setup ---

type dbTestEnv struct {
	server *httptest.Server
	mgr    *dbMockManager
}

func setupDBTestServer(t *testing.T) *dbTestEnv {
	t.Helper()

	if testPool == nil {
		t.Skip("skipping: test database not available")
	}

	// Truncate for clean slate
	_, err := testPool.Exec(context.Background(), "TRUNCATE TABLE databases CASCADE")
	require.NoError(t, err)

	repo := database.NewRepository(testPool)
	mgr := &dbMockManager{}

	checker := &mockHealthChecker{
		status: k8s.ConnectivityStatus{Connected: true, Version: "v1.31.0"},
	}
	pinger := &dbTestPinger{pool: testPool}

	router := api.NewRouter(api.RouterDeps{
		K8sChecker: checker,
		DBPinger:   pinger,
		Version:    "0.1.0-test",
		Repo:       repo,
		K8sManager: mgr,
		Namespace:  "default",
	})

	server := httptest.NewServer(router)
	t.Cleanup(func() { server.Close() })

	return &dbTestEnv{server: server, mgr: mgr}
}

// --- HTTP helper ---

func dbDoRequest(t *testing.T, method, url string, body interface{}) (*http.Response, map[string]interface{}) {
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
		"purpose":   "integration test",
	}

	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", createBody)
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

	// Verify K8s resources were applied
	assert.Equal(t, 1, env.mgr.appliedClusters)
	assert.Equal(t, 1, env.mgr.appliedPoolers)

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
	resp, result = dbDoRequest(t, http.MethodGet, env.server.URL+"/databases", nil)
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
	resp, result = dbDoRequest(t, http.MethodGet, env.server.URL+"/databases/"+dbID, nil)
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
	resp, result = dbDoRequest(t, http.MethodPatch, env.server.URL+"/databases/"+dbID, updateBody)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	data = result["data"].(map[string]interface{})
	assert.Equal(t, "backend", data["ownerTeam"])
	assert.Equal(t, "updated purpose", data["purpose"])
	assert.Equal(t, "lifecycle-db", data["name"])

	// Verify update persisted via GET
	resp, result = dbDoRequest(t, http.MethodGet, env.server.URL+"/databases/"+dbID, nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	data = result["data"].(map[string]interface{})
	assert.Equal(t, "backend", data["ownerTeam"])
	assert.Equal(t, "updated purpose", data["purpose"])

	// Step 5: DELETE /databases/:id -> 204
	resp, _ = dbDoRequest(t, http.MethodDelete, env.server.URL+"/databases/"+dbID, nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	assert.Equal(t, 1, env.mgr.deletedClusters)
	assert.Equal(t, 1, env.mgr.deletedPoolers)

	// Step 6: GET /databases/:id -> 404 (soft-deleted)
	resp, result = dbDoRequest(t, http.MethodGet, env.server.URL+"/databases/"+dbID, nil)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	errObj := result["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])

	// Step 7: GET /databases -> empty list
	resp, result = dbDoRequest(t, http.MethodGet, env.server.URL+"/databases", nil)
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
	})
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
	})
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
		}
		resp, _ := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	}

	// Page 1, limit 2 -> 2 results, total=3
	resp, result := dbDoRequest(t, http.MethodGet, env.server.URL+"/databases?page=1&limit=2", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	listData := result["data"].([]interface{})
	assert.Len(t, listData, 2)

	meta := result["meta"].(map[string]interface{})
	assert.Equal(t, float64(3), meta["total"])
	assert.Equal(t, float64(1), meta["page"])
	assert.Equal(t, float64(2), meta["limit"])

	// Page 2, limit 2 -> 1 result
	resp, result = dbDoRequest(t, http.MethodGet, env.server.URL+"/databases?page=2&limit=2", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	listData = result["data"].([]interface{})
	assert.Len(t, listData, 1)
}

// ===== Filter by Team =====

func TestList_FilterByTeam(t *testing.T) {
	env := setupDBTestServer(t)

	// Create databases with different owner teams
	for _, team := range []string{"alpha", "alpha", "beta"} {
		body := map[string]interface{}{
			"name":      fmt.Sprintf("filter-%s-%d", team, env.mgr.appliedClusters),
			"ownerTeam": team,
		}
		resp, _ := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
	}

	// Filter by alpha -> 2 results
	resp, result := dbDoRequest(t, http.MethodGet, env.server.URL+"/databases?owner_team=alpha", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	listData := result["data"].([]interface{})
	assert.Len(t, listData, 2)
	for _, item := range listData {
		db := item.(map[string]interface{})
		assert.Equal(t, "alpha", db["ownerTeam"])
	}

	// Filter by beta -> 1 result
	resp, result = dbDoRequest(t, http.MethodGet, env.server.URL+"/databases?owner_team=beta", nil)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	listData = result["data"].([]interface{})
	assert.Len(t, listData, 1)
	assert.Equal(t, "beta", listData[0].(map[string]interface{})["ownerTeam"])

	// Filter by nonexistent -> 0 results
	resp, result = dbDoRequest(t, http.MethodGet, env.server.URL+"/databases?owner_team=gamma", nil)
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
	}

	resp, _ := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body)
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
	}

	resp, result := dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	dbID := result["data"].(map[string]interface{})["id"].(string)

	resp, _ = dbDoRequest(t, http.MethodDelete, env.server.URL+"/databases/"+dbID, nil)
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)

	resp, result = dbDoRequest(t, http.MethodPost, env.server.URL+"/databases", body)
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	newID := result["data"].(map[string]interface{})["id"].(string)
	assert.NotEqual(t, dbID, newID)
}
