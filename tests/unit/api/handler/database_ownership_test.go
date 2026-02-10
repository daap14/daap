package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daap14/daap/internal/api/middleware"
	"github.com/daap14/daap/internal/auth"
	"github.com/daap14/daap/internal/database"
)

// --- Helpers for identity-aware requests ---

func makeAuthRequest(method, path string, body []byte, params map[string]string, identity *auth.Identity) (*http.Request, *httptest.ResponseRecorder) {
	req, w := makeChiRequest(method, path, body, "", params)
	if identity != nil {
		ctx := middleware.WithIdentity(req.Context(), identity)
		req = req.WithContext(ctx)
	}
	return req, w
}

func productIdentity(teamName string) *auth.Identity {
	teamID := uuid.New()
	role := "product"
	return &auth.Identity{
		UserID:      uuid.New(),
		UserName:    "product-user",
		TeamID:      &teamID,
		TeamName:    &teamName,
		Role:        &role,
		IsSuperuser: false,
	}
}

func platformIdentity() *auth.Identity {
	teamID := uuid.New()
	teamName := "platform-ops"
	role := "platform"
	return &auth.Identity{
		UserID:      uuid.New(),
		UserName:    "platform-user",
		TeamID:      &teamID,
		TeamName:    &teamName,
		Role:        &role,
		IsSuperuser: false,
	}
}

// ===== POST /databases — Ownership Scoping =====

func TestCreate_ProductUser_AutoSetsOwnerTeam(t *testing.T) {
	t.Parallel()

	var capturedDB *database.Database
	repo := &mockRepo{
		createFn: func(_ context.Context, db *database.Database) error {
			capturedDB = db
			db.ID = uuid.New()
			db.ClusterName = "daap-" + db.Name
			db.PoolerName = "daap-" + db.Name + "-pooler"
			db.Status = "provisioning"
			return nil
		},
	}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	body, _ := json.Marshal(map[string]interface{}{
		"name": "mydb",
	})

	identity := productIdentity("my-team")
	req, w := makeAuthRequest(http.MethodPost, "/databases", body, nil, identity)

	h.Create(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	require.NotNil(t, capturedDB)
	assert.Equal(t, "my-team", capturedDB.OwnerTeam)
}

func TestCreate_ProductUser_MatchingOwnerTeamAllowed(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	body, _ := json.Marshal(map[string]interface{}{
		"name":      "mydb",
		"ownerTeam": "my-team",
	})

	identity := productIdentity("my-team")
	req, w := makeAuthRequest(http.MethodPost, "/databases", body, nil, identity)

	h.Create(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestCreate_ProductUser_MismatchedOwnerTeamForbidden(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	body, _ := json.Marshal(map[string]interface{}{
		"name":      "mydb",
		"ownerTeam": "other-team",
	})

	identity := productIdentity("my-team")
	req, w := makeAuthRequest(http.MethodPost, "/databases", body, nil, identity)

	h.Create(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "FORBIDDEN", errObj["code"])
}

func TestCreate_PlatformUser_AnyOwnerTeamAllowed(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	body, _ := json.Marshal(map[string]interface{}{
		"name":      "mydb",
		"ownerTeam": "any-team",
	})

	identity := platformIdentity()
	req, w := makeAuthRequest(http.MethodPost, "/databases", body, nil, identity)

	h.Create(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

// ===== GET /databases (List) — Ownership Scoping =====

func TestList_ProductUser_AutoFiltersOwnerTeam(t *testing.T) {
	t.Parallel()

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
	h := newTestHandler(repo, mgr)

	identity := productIdentity("my-team")
	req, w := makeAuthRequest(http.MethodGet, "/databases", nil, nil, identity)

	h.List(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, capturedFilter.OwnerTeam)
	assert.Equal(t, "my-team", *capturedFilter.OwnerTeam)
}

func TestList_PlatformUser_SeesAll(t *testing.T) {
	t.Parallel()

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
	h := newTestHandler(repo, mgr)

	identity := platformIdentity()
	req, w := makeAuthRequest(http.MethodGet, "/databases", nil, nil, identity)

	h.List(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Nil(t, capturedFilter.OwnerTeam)
}

// ===== GET /databases/{id} — Ownership Scoping =====

func TestGetByID_ProductUser_OwnDatabase(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*database.Database, error) {
			db := sampleDB(id, "ready")
			db.OwnerTeam = "my-team"
			return db, nil
		},
	}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	identity := productIdentity("my-team")
	req, w := makeAuthRequest(http.MethodGet, "/databases/"+id.String(), nil, map[string]string{"id": id.String()}, identity)

	h.GetByID(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetByID_ProductUser_OtherTeamReturns404(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*database.Database, error) {
			db := sampleDB(id, "ready")
			db.OwnerTeam = "other-team"
			return db, nil
		},
	}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	identity := productIdentity("my-team")
	req, w := makeAuthRequest(http.MethodGet, "/databases/"+id.String(), nil, map[string]string{"id": id.String()}, identity)

	h.GetByID(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

func TestGetByID_PlatformUser_SeesAll(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*database.Database, error) {
			db := sampleDB(id, "ready")
			db.OwnerTeam = "any-team"
			return db, nil
		},
	}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	identity := platformIdentity()
	req, w := makeAuthRequest(http.MethodGet, "/databases/"+id.String(), nil, map[string]string{"id": id.String()}, identity)

	h.GetByID(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ===== PATCH /databases/{id} — Ownership Scoping =====

func TestUpdate_ProductUser_CannotChangeOwnerTeam(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockRepo{}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	body, _ := json.Marshal(map[string]interface{}{
		"ownerTeam": "new-team",
	})

	identity := productIdentity("my-team")
	req, w := makeAuthRequest(http.MethodPatch, "/databases/"+id.String(), body, map[string]string{"id": id.String()}, identity)

	h.Update(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "FORBIDDEN", errObj["code"])
}

func TestUpdate_ProductUser_OwnDatabaseAllowed(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*database.Database, error) {
			db := sampleDB(id, "provisioning")
			db.OwnerTeam = "my-team"
			return db, nil
		},
		updateFn: func(_ context.Context, _ uuid.UUID, fields database.UpdateFields) (*database.Database, error) {
			db := sampleDB(id, "provisioning")
			db.OwnerTeam = "my-team"
			if fields.Purpose != nil {
				db.Purpose = *fields.Purpose
			}
			return db, nil
		},
	}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	body, _ := json.Marshal(map[string]interface{}{
		"purpose": "updated",
	})

	identity := productIdentity("my-team")
	req, w := makeAuthRequest(http.MethodPatch, "/databases/"+id.String(), body, map[string]string{"id": id.String()}, identity)

	h.Update(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestUpdate_ProductUser_OtherTeamReturns404(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*database.Database, error) {
			db := sampleDB(id, "provisioning")
			db.OwnerTeam = "other-team"
			return db, nil
		},
	}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	body, _ := json.Marshal(map[string]interface{}{
		"purpose": "updated",
	})

	identity := productIdentity("my-team")
	req, w := makeAuthRequest(http.MethodPatch, "/databases/"+id.String(), body, map[string]string{"id": id.String()}, identity)

	h.Update(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestUpdate_PlatformUser_CanChangeOwnerTeam(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockRepo{
		updateFn: func(_ context.Context, _ uuid.UUID, fields database.UpdateFields) (*database.Database, error) {
			db := sampleDB(id, "provisioning")
			if fields.OwnerTeam != nil {
				db.OwnerTeam = *fields.OwnerTeam
			}
			return db, nil
		},
	}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	body, _ := json.Marshal(map[string]interface{}{
		"ownerTeam": "new-team",
	})

	identity := platformIdentity()
	req, w := makeAuthRequest(http.MethodPatch, "/databases/"+id.String(), body, map[string]string{"id": id.String()}, identity)

	h.Update(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ===== DELETE /databases/{id} — Ownership Scoping =====

func TestDelete_ProductUser_OwnDatabaseAllowed(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*database.Database, error) {
			db := sampleDB(id, "provisioning")
			db.OwnerTeam = "my-team"
			return db, nil
		},
	}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	identity := productIdentity("my-team")
	req, w := makeAuthRequest(http.MethodDelete, "/databases/"+id.String(), nil, map[string]string{"id": id.String()}, identity)

	h.Delete(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestDelete_ProductUser_OtherTeamReturns404(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*database.Database, error) {
			db := sampleDB(id, "provisioning")
			db.OwnerTeam = "other-team"
			return db, nil
		},
	}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	identity := productIdentity("my-team")
	req, w := makeAuthRequest(http.MethodDelete, "/databases/"+id.String(), nil, map[string]string{"id": id.String()}, identity)

	h.Delete(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

func TestDelete_PlatformUser_DeletesAnyTeam(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*database.Database, error) {
			db := sampleDB(id, "provisioning")
			db.OwnerTeam = "any-team"
			return db, nil
		},
	}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	identity := platformIdentity()
	req, w := makeAuthRequest(http.MethodDelete, "/databases/"+id.String(), nil, map[string]string{"id": id.String()}, identity)

	h.Delete(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}
