package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/daap14/daap/internal/api/handler"
	"github.com/daap14/daap/internal/database"
	"github.com/daap14/daap/internal/k8s"
	"github.com/daap14/daap/internal/team"
)

// --- Mock Repository ---

type mockRepo struct {
	createFn       func(ctx context.Context, db *database.Database) error
	getByIDFn      func(ctx context.Context, id uuid.UUID) (*database.Database, error)
	listFn         func(ctx context.Context, filter database.ListFilter) (*database.ListResult, error)
	updateFn       func(ctx context.Context, id uuid.UUID, fields database.UpdateFields) (*database.Database, error)
	updateStatusFn func(ctx context.Context, id uuid.UUID, su database.StatusUpdate) (*database.Database, error)
	softDeleteFn   func(ctx context.Context, id uuid.UUID) error
}

func (m *mockRepo) Create(ctx context.Context, db *database.Database) error {
	if m.createFn != nil {
		return m.createFn(ctx, db)
	}
	db.ID = uuid.New()
	db.ClusterName = fmt.Sprintf("daap-%s", db.Name)
	db.PoolerName = fmt.Sprintf("daap-%s-pooler", db.Name)
	db.Status = "provisioning"
	db.CreatedAt = time.Now().UTC()
	db.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *mockRepo) GetByID(ctx context.Context, id uuid.UUID) (*database.Database, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, database.ErrNotFound
}

func (m *mockRepo) List(ctx context.Context, filter database.ListFilter) (*database.ListResult, error) {
	if m.listFn != nil {
		return m.listFn(ctx, filter)
	}
	return &database.ListResult{Databases: []database.Database{}, Total: 0, Page: 1, Limit: 20}, nil
}

func (m *mockRepo) Update(ctx context.Context, id uuid.UUID, fields database.UpdateFields) (*database.Database, error) {
	if m.updateFn != nil {
		return m.updateFn(ctx, id, fields)
	}
	return nil, database.ErrNotFound
}

func (m *mockRepo) UpdateStatus(ctx context.Context, id uuid.UUID, su database.StatusUpdate) (*database.Database, error) {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, id, su)
	}
	return nil, database.ErrNotFound
}

func (m *mockRepo) SoftDelete(ctx context.Context, id uuid.UUID) error {
	if m.softDeleteFn != nil {
		return m.softDeleteFn(ctx, id)
	}
	return nil
}

// --- Mock ResourceManager ---

type mockManager struct {
	applyClusterFn     func(ctx context.Context, cluster *unstructured.Unstructured) error
	applyPoolerFn      func(ctx context.Context, pooler *unstructured.Unstructured) error
	deleteClusterFn    func(ctx context.Context, namespace, name string) error
	deletePoolerFn     func(ctx context.Context, namespace, name string) error
	getClusterStatusFn func(ctx context.Context, namespace, name string) (k8s.ClusterStatus, error)
	getSecretFn        func(ctx context.Context, namespace, name string) (map[string][]byte, error)
}

func (m *mockManager) ApplyCluster(ctx context.Context, cluster *unstructured.Unstructured) error {
	if m.applyClusterFn != nil {
		return m.applyClusterFn(ctx, cluster)
	}
	return nil
}

func (m *mockManager) ApplyPooler(ctx context.Context, pooler *unstructured.Unstructured) error {
	if m.applyPoolerFn != nil {
		return m.applyPoolerFn(ctx, pooler)
	}
	return nil
}

func (m *mockManager) DeleteCluster(ctx context.Context, namespace, name string) error {
	if m.deleteClusterFn != nil {
		return m.deleteClusterFn(ctx, namespace, name)
	}
	return nil
}

func (m *mockManager) DeletePooler(ctx context.Context, namespace, name string) error {
	if m.deletePoolerFn != nil {
		return m.deletePoolerFn(ctx, namespace, name)
	}
	return nil
}

func (m *mockManager) GetClusterStatus(ctx context.Context, namespace, name string) (k8s.ClusterStatus, error) {
	if m.getClusterStatusFn != nil {
		return m.getClusterStatusFn(ctx, namespace, name)
	}
	return k8s.ClusterStatus{}, nil
}

func (m *mockManager) GetSecret(ctx context.Context, namespace, name string) (map[string][]byte, error) {
	if m.getSecretFn != nil {
		return m.getSecretFn(ctx, namespace, name)
	}
	return nil, nil
}

// --- Mock Team Repository ---

type mockDBTeamRepo struct {
	getByNameFn func(ctx context.Context, name string) (*team.Team, error)
	getByIDFn   func(ctx context.Context, id uuid.UUID) (*team.Team, error)
	createFn    func(ctx context.Context, t *team.Team) error
	listFn      func(ctx context.Context) ([]team.Team, error)
	deleteFn    func(ctx context.Context, id uuid.UUID) error
}

func (m *mockDBTeamRepo) Create(ctx context.Context, t *team.Team) error {
	if m.createFn != nil {
		return m.createFn(ctx, t)
	}
	return nil
}

func (m *mockDBTeamRepo) GetByID(ctx context.Context, id uuid.UUID) (*team.Team, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, team.ErrTeamNotFound
}

func (m *mockDBTeamRepo) GetByName(ctx context.Context, name string) (*team.Team, error) {
	if m.getByNameFn != nil {
		return m.getByNameFn(ctx, name)
	}
	return &team.Team{
		ID:   uuid.New(),
		Name: name,
		Role: "platform",
	}, nil
}

func (m *mockDBTeamRepo) List(ctx context.Context) ([]team.Team, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return []team.Team{}, nil
}

func (m *mockDBTeamRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

// --- Helpers ---

func newTestHandler(repo database.Repository, mgr k8s.ResourceManager, teamRepo team.Repository) *handler.DatabaseHandler {
	return handler.NewDatabaseHandler(repo, mgr, teamRepo, "default")
}

func makeChiRequest(method, path string, body []byte, routePattern string, params map[string]string) (*http.Request, *httptest.ResponseRecorder) {
	var req *http.Request
	if body != nil {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	if len(params) > 0 {
		rctx := chi.NewRouteContext()
		for k, v := range params {
			rctx.URLParams.Add(k, v)
		}
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	}

	return req, w
}

func parseEnvelope(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err, "failed to parse response body")
	return env
}

// platformTeamID is a shared team ID for tests that use platform as the owner
var platformTeamID = uuid.New()

func sampleDB(id uuid.UUID, status string) *database.Database {
	now := time.Now().UTC()
	db := &database.Database{
		ID:            id,
		Name:          "testdb",
		OwnerTeamID:   platformTeamID,
		OwnerTeamName: "platform",
		Purpose:       "testing",
		Namespace:     "default",
		ClusterName:   "daap-testdb",
		PoolerName:    "daap-testdb-pooler",
		Status:        status,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if status == "ready" {
		host := "daap-testdb-pooler.default.svc.cluster.local"
		port := 5432
		secretName := "daap-testdb-app"
		db.Host = &host
		db.Port = &port
		db.SecretName = &secretName
	}
	return db
}

// ===== POST /databases =====

func TestCreate_Success(t *testing.T) {
	// Arrange
	repo := &mockRepo{}
	mgr := &mockManager{}
	teamRepo := &mockDBTeamRepo{}
	h := newTestHandler(repo, mgr, teamRepo)

	body, _ := json.Marshal(map[string]interface{}{
		"name":      "mydb",
		"ownerTeam": "platform",
		"tier":      "standard",
		"purpose":   "testing",
	})

	req, w := makeChiRequest(http.MethodPost, "/databases", body, "/databases", nil)

	// Act
	h.Create(w, req)

	// Assert
	assert.Equal(t, http.StatusCreated, w.Code)

	env := parseEnvelope(t, w)
	assert.Nil(t, env["error"])
	assert.NotNil(t, env["data"])
	assert.NotNil(t, env["meta"])

	data := env["data"].(map[string]interface{})
	assert.Equal(t, "provisioning", data["status"])
	assert.NotEmpty(t, data["id"])
	assert.NotEmpty(t, data["createdAt"])
}

func TestCreate_ValidationError(t *testing.T) {
	// Arrange
	repo := &mockRepo{}
	mgr := &mockManager{}
	teamRepo := &mockDBTeamRepo{}
	h := newTestHandler(repo, mgr, teamRepo)

	body, _ := json.Marshal(map[string]interface{}{
		"name":      "X", // too short and uppercase
		"ownerTeam": "",
	})

	req, w := makeChiRequest(http.MethodPost, "/databases", body, "/databases", nil)

	// Act
	h.Create(w, req)

	// Assert
	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := parseEnvelope(t, w)
	assert.NotNil(t, env["error"])
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])
	assert.NotNil(t, errObj["details"])
}

func TestCreate_DuplicateName(t *testing.T) {
	// Arrange
	repo := &mockRepo{
		createFn: func(_ context.Context, _ *database.Database) error {
			return database.ErrDuplicateName
		},
	}
	mgr := &mockManager{}
	teamRepo := &mockDBTeamRepo{}
	h := newTestHandler(repo, mgr, teamRepo)

	body, _ := json.Marshal(map[string]interface{}{
		"name":      "existing-db",
		"ownerTeam": "platform",
		"tier":      "standard",
	})

	req, w := makeChiRequest(http.MethodPost, "/databases", body, "/databases", nil)

	// Act
	h.Create(w, req)

	// Assert
	assert.Equal(t, http.StatusConflict, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "DUPLICATE_NAME", errObj["code"])
}

func TestCreate_OwnerTeamNotFound(t *testing.T) {
	// Arrange
	repo := &mockRepo{}
	mgr := &mockManager{}
	teamRepo := &mockDBTeamRepo{
		getByNameFn: func(_ context.Context, _ string) (*team.Team, error) {
			return nil, team.ErrTeamNotFound
		},
	}
	h := newTestHandler(repo, mgr, teamRepo)

	body, _ := json.Marshal(map[string]interface{}{
		"name":      "mydb",
		"ownerTeam": "nonexistent-team",
		"tier":      "standard",
	})

	req, w := makeChiRequest(http.MethodPost, "/databases", body, "/databases", nil)

	// Act
	h.Create(w, req)

	// Assert
	assert.Equal(t, http.StatusNotFound, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

func TestCreate_K8sError_MarksRecordAsError(t *testing.T) {
	// Arrange: repo succeeds, K8s ApplyCluster fails
	var statusUpdate *database.StatusUpdate
	repo := &mockRepo{
		updateStatusFn: func(_ context.Context, _ uuid.UUID, su database.StatusUpdate) (*database.Database, error) {
			statusUpdate = &su
			return &database.Database{Status: su.Status}, nil
		},
	}
	mgr := &mockManager{
		applyClusterFn: func(_ context.Context, _ *unstructured.Unstructured) error {
			return errors.New("k8s connection refused")
		},
	}
	teamRepo := &mockDBTeamRepo{}
	h := newTestHandler(repo, mgr, teamRepo)

	body, _ := json.Marshal(map[string]interface{}{
		"name":      "mydb",
		"ownerTeam": "platform",
		"tier":      "standard",
	})

	req, w := makeChiRequest(http.MethodPost, "/databases", body, "/databases", nil)

	// Act
	h.Create(w, req)

	// Assert: returns 500 with K8S_ERROR
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "K8S_ERROR", errObj["code"])

	// Assert: record was marked as "error"
	require.NotNil(t, statusUpdate, "expected UpdateStatus to be called")
	assert.Equal(t, "error", statusUpdate.Status)
}

// ===== GET /databases =====

func TestList_Success(t *testing.T) {
	// Arrange
	id1 := uuid.New()
	id2 := uuid.New()
	repo := &mockRepo{
		listFn: func(_ context.Context, filter database.ListFilter) (*database.ListResult, error) {
			return &database.ListResult{
				Databases: []database.Database{
					*sampleDB(id1, "provisioning"),
					*sampleDB(id2, "ready"),
				},
				Total: 2,
				Page:  filter.Page,
				Limit: filter.Limit,
			}, nil
		},
	}
	mgr := &mockManager{}
	teamRepo := &mockDBTeamRepo{}
	h := newTestHandler(repo, mgr, teamRepo)

	req, w := makeChiRequest(http.MethodGet, "/databases", nil, "/databases", nil)

	// Act
	h.List(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)

	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err)

	data := env["data"].([]interface{})
	assert.Len(t, data, 2)

	meta := env["meta"].(map[string]interface{})
	assert.Equal(t, float64(2), meta["total"])
	assert.NotNil(t, meta["page"])
	assert.NotNil(t, meta["limit"])
}

func TestList_WithFilters(t *testing.T) {
	// Arrange
	filterTeamID := uuid.New()
	var capturedFilter database.ListFilter
	repo := &mockRepo{
		listFn: func(_ context.Context, filter database.ListFilter) (*database.ListResult, error) {
			capturedFilter = filter
			return &database.ListResult{
				Databases: []database.Database{},
				Total:     0,
				Page:      filter.Page,
				Limit:     filter.Limit,
			}, nil
		},
	}
	mgr := &mockManager{}
	teamRepo := &mockDBTeamRepo{
		getByNameFn: func(_ context.Context, name string) (*team.Team, error) {
			return &team.Team{ID: filterTeamID, Name: name, Role: "platform"}, nil
		},
	}
	h := newTestHandler(repo, mgr, teamRepo)

	req, w := makeChiRequest(http.MethodGet, "/databases?owner_team=platform&status=ready&name=test", nil, "/databases", nil)

	// Act
	h.List(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, capturedFilter.OwnerTeamID)
	assert.Equal(t, filterTeamID, *capturedFilter.OwnerTeamID)
	require.NotNil(t, capturedFilter.Status)
	assert.Equal(t, "ready", *capturedFilter.Status)
	require.NotNil(t, capturedFilter.Name)
	assert.Equal(t, "test", *capturedFilter.Name)
}

func TestList_DefaultPagination(t *testing.T) {
	// Arrange
	var capturedFilter database.ListFilter
	repo := &mockRepo{
		listFn: func(_ context.Context, filter database.ListFilter) (*database.ListResult, error) {
			capturedFilter = filter
			return &database.ListResult{
				Databases: []database.Database{},
				Total:     0,
				Page:      filter.Page,
				Limit:     filter.Limit,
			}, nil
		},
	}
	mgr := &mockManager{}
	teamRepo := &mockDBTeamRepo{}
	h := newTestHandler(repo, mgr, teamRepo)

	req, w := makeChiRequest(http.MethodGet, "/databases", nil, "/databases", nil)

	// Act
	h.List(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 1, capturedFilter.Page)
	assert.Equal(t, 20, capturedFilter.Limit)
}

func TestList_ConnectionDetailsOnlyForReady(t *testing.T) {
	// Arrange
	id1 := uuid.New()
	id2 := uuid.New()
	repo := &mockRepo{
		listFn: func(_ context.Context, filter database.ListFilter) (*database.ListResult, error) {
			return &database.ListResult{
				Databases: []database.Database{
					*sampleDB(id1, "provisioning"),
					*sampleDB(id2, "ready"),
				},
				Total: 2,
				Page:  filter.Page,
				Limit: filter.Limit,
			}, nil
		},
	}
	mgr := &mockManager{}
	teamRepo := &mockDBTeamRepo{}
	h := newTestHandler(repo, mgr, teamRepo)

	req, w := makeChiRequest(http.MethodGet, "/databases", nil, "/databases", nil)

	// Act
	h.List(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)

	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err)

	data := env["data"].([]interface{})

	// First is provisioning - should NOT have host/port
	provisioning := data[0].(map[string]interface{})
	assert.Equal(t, "provisioning", provisioning["status"])
	assert.Nil(t, provisioning["host"])
	assert.Nil(t, provisioning["port"])

	// Second is ready - SHOULD have host/port/secretName
	ready := data[1].(map[string]interface{})
	assert.Equal(t, "ready", ready["status"])
	assert.NotNil(t, ready["host"])
	assert.NotNil(t, ready["port"])
	assert.NotNil(t, ready["secretName"])
	assert.Nil(t, ready["username"], "username should not be in response")
	assert.Nil(t, ready["password"], "password should not be in response")
}

// ===== GET /databases/:id =====

func TestGetByID_Success(t *testing.T) {
	// Arrange
	id := uuid.New()
	repo := &mockRepo{
		getByIDFn: func(_ context.Context, reqID uuid.UUID) (*database.Database, error) {
			assert.Equal(t, id, reqID)
			return sampleDB(id, "provisioning"), nil
		},
	}
	mgr := &mockManager{}
	teamRepo := &mockDBTeamRepo{}
	h := newTestHandler(repo, mgr, teamRepo)

	req, w := makeChiRequest(http.MethodGet, "/databases/"+id.String(), nil, "/databases/{id}", map[string]string{"id": id.String()})

	// Act
	h.GetByID(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)

	env := parseEnvelope(t, w)
	assert.Nil(t, env["error"])
	data := env["data"].(map[string]interface{})
	assert.Equal(t, id.String(), data["id"])
}

func TestGetByID_NotFound(t *testing.T) {
	// Arrange
	id := uuid.New()
	repo := &mockRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*database.Database, error) {
			return nil, database.ErrNotFound
		},
	}
	mgr := &mockManager{}
	teamRepo := &mockDBTeamRepo{}
	h := newTestHandler(repo, mgr, teamRepo)

	req, w := makeChiRequest(http.MethodGet, "/databases/"+id.String(), nil, "/databases/{id}", map[string]string{"id": id.String()})

	// Act
	h.GetByID(w, req)

	// Assert
	assert.Equal(t, http.StatusNotFound, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

func TestGetByID_InvalidUUID(t *testing.T) {
	// Arrange
	repo := &mockRepo{}
	mgr := &mockManager{}
	teamRepo := &mockDBTeamRepo{}
	h := newTestHandler(repo, mgr, teamRepo)

	req, w := makeChiRequest(http.MethodGet, "/databases/not-a-uuid", nil, "/databases/{id}", map[string]string{"id": "not-a-uuid"})

	// Act
	h.GetByID(w, req)

	// Assert
	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_ID", errObj["code"])
}

func TestGetByID_ConnectionDetailsWhenReady(t *testing.T) {
	// Arrange
	id := uuid.New()
	repo := &mockRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*database.Database, error) {
			return sampleDB(id, "ready"), nil
		},
	}
	mgr := &mockManager{}
	teamRepo := &mockDBTeamRepo{}
	h := newTestHandler(repo, mgr, teamRepo)

	req, w := makeChiRequest(http.MethodGet, "/databases/"+id.String(), nil, "/databases/{id}", map[string]string{"id": id.String()})

	// Act
	h.GetByID(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)

	env := parseEnvelope(t, w)
	data := env["data"].(map[string]interface{})
	assert.Equal(t, "ready", data["status"])
	assert.NotNil(t, data["host"])
	assert.NotNil(t, data["port"])
	assert.NotNil(t, data["secretName"])
	assert.Nil(t, data["username"], "username should not be in response")
	assert.Nil(t, data["password"], "password should not be in response")
}

// ===== PATCH /databases/:id =====

func TestUpdate_Success(t *testing.T) {
	// Arrange
	id := uuid.New()
	newTeamID := uuid.New()
	repo := &mockRepo{
		updateFn: func(_ context.Context, reqID uuid.UUID, fields database.UpdateFields) (*database.Database, error) {
			assert.Equal(t, id, reqID)
			db := sampleDB(id, "provisioning")
			if fields.OwnerTeamID != nil {
				db.OwnerTeamID = *fields.OwnerTeamID
				db.OwnerTeamName = "new-team"
			}
			if fields.Purpose != nil {
				db.Purpose = *fields.Purpose
			}
			return db, nil
		},
	}
	mgr := &mockManager{}
	teamRepo := &mockDBTeamRepo{
		getByNameFn: func(_ context.Context, name string) (*team.Team, error) {
			return &team.Team{ID: newTeamID, Name: name, Role: "platform"}, nil
		},
	}
	h := newTestHandler(repo, mgr, teamRepo)

	body, _ := json.Marshal(map[string]interface{}{
		"ownerTeam": "new-team",
		"purpose":   "updated purpose",
	})
	req, w := makeChiRequest(http.MethodPatch, "/databases/"+id.String(), body, "/databases/{id}", map[string]string{"id": id.String()})

	// Act
	h.Update(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)

	env := parseEnvelope(t, w)
	assert.Nil(t, env["error"])
	data := env["data"].(map[string]interface{})
	assert.Equal(t, "new-team", data["ownerTeam"])
}

func TestUpdate_NameImmutable(t *testing.T) {
	// Arrange
	id := uuid.New()
	repo := &mockRepo{}
	mgr := &mockManager{}
	teamRepo := &mockDBTeamRepo{}
	h := newTestHandler(repo, mgr, teamRepo)

	body, _ := json.Marshal(map[string]interface{}{
		"name": "new-name",
	})
	req, w := makeChiRequest(http.MethodPatch, "/databases/"+id.String(), body, "/databases/{id}", map[string]string{"id": id.String()})

	// Act
	h.Update(w, req)

	// Assert
	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "IMMUTABLE_FIELD", errObj["code"])
}

func TestUpdate_NotFound(t *testing.T) {
	// Arrange
	id := uuid.New()
	repo := &mockRepo{
		updateFn: func(_ context.Context, _ uuid.UUID, _ database.UpdateFields) (*database.Database, error) {
			return nil, database.ErrNotFound
		},
	}
	mgr := &mockManager{}
	teamRepo := &mockDBTeamRepo{}
	h := newTestHandler(repo, mgr, teamRepo)

	body, _ := json.Marshal(map[string]interface{}{
		"purpose": "updated",
	})
	req, w := makeChiRequest(http.MethodPatch, "/databases/"+id.String(), body, "/databases/{id}", map[string]string{"id": id.String()})

	// Act
	h.Update(w, req)

	// Assert
	assert.Equal(t, http.StatusNotFound, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

// ===== DELETE /databases/:id =====

func TestDelete_Success(t *testing.T) {
	// Arrange
	id := uuid.New()
	deleteClusterCalled := false
	deletePoolerCalled := false
	softDeleteCalled := false

	repo := &mockRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*database.Database, error) {
			return sampleDB(id, "provisioning"), nil
		},
		softDeleteFn: func(_ context.Context, _ uuid.UUID) error {
			softDeleteCalled = true
			return nil
		},
	}
	mgr := &mockManager{
		deleteClusterFn: func(_ context.Context, _, _ string) error {
			deleteClusterCalled = true
			return nil
		},
		deletePoolerFn: func(_ context.Context, _, _ string) error {
			deletePoolerCalled = true
			return nil
		},
	}
	teamRepo := &mockDBTeamRepo{}
	h := newTestHandler(repo, mgr, teamRepo)

	req, w := makeChiRequest(http.MethodDelete, "/databases/"+id.String(), nil, "/databases/{id}", map[string]string{"id": id.String()})

	// Act
	h.Delete(w, req)

	// Assert
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.True(t, deleteClusterCalled, "expected DeleteCluster to be called")
	assert.True(t, deletePoolerCalled, "expected DeletePooler to be called")
	assert.True(t, softDeleteCalled, "expected SoftDelete to be called")
}

func TestDelete_NotFound(t *testing.T) {
	// Arrange
	id := uuid.New()
	repo := &mockRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*database.Database, error) {
			return nil, database.ErrNotFound
		},
	}
	mgr := &mockManager{}
	teamRepo := &mockDBTeamRepo{}
	h := newTestHandler(repo, mgr, teamRepo)

	req, w := makeChiRequest(http.MethodDelete, "/databases/"+id.String(), nil, "/databases/{id}", map[string]string{"id": id.String()})

	// Act
	h.Delete(w, req)

	// Assert
	assert.Equal(t, http.StatusNotFound, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

func TestDelete_K8sCleanupIgnoresNotFound(t *testing.T) {
	// Arrange: K8s delete returns errors, but soft-delete should still proceed
	id := uuid.New()
	softDeleteCalled := false

	repo := &mockRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*database.Database, error) {
			return sampleDB(id, "provisioning"), nil
		},
		softDeleteFn: func(_ context.Context, _ uuid.UUID) error {
			softDeleteCalled = true
			return nil
		},
	}
	mgr := &mockManager{
		deleteClusterFn: func(_ context.Context, _, _ string) error {
			return errors.New("cluster not found")
		},
		deletePoolerFn: func(_ context.Context, _, _ string) error {
			return errors.New("pooler not found")
		},
	}
	teamRepo := &mockDBTeamRepo{}
	h := newTestHandler(repo, mgr, teamRepo)

	req, w := makeChiRequest(http.MethodDelete, "/databases/"+id.String(), nil, "/databases/{id}", map[string]string{"id": id.String()})

	// Act
	h.Delete(w, req)

	// Assert: still returns 204 because K8s errors are logged but not fatal
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.True(t, softDeleteCalled, "expected SoftDelete to still be called despite K8s errors")
}
