package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daap14/daap/internal/api/middleware"
	"github.com/daap14/daap/internal/auth"
	"github.com/daap14/daap/internal/database"
)

// --- Identity Helpers ---

func productIdentity(teamName string) *auth.Identity {
	role := "product"
	teamID := uuid.New()
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
	role := "platform"
	teamID := uuid.New()
	teamName := "ops"
	return &auth.Identity{
		UserID:      uuid.New(),
		UserName:    "platform-user",
		TeamID:      &teamID,
		TeamName:    &teamName,
		Role:        &role,
		IsSuperuser: false,
	}
}

func withIdentityRequest(req *http.Request, identity *auth.Identity) *http.Request {
	ctx := middleware.WithIdentity(req.Context(), identity)
	return req.WithContext(ctx)
}

// ===== POST /databases — Ownership =====

func TestCreate_ProductUser_AutoFillOwnerTeam(t *testing.T) {
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
		"name":    "mydb",
		"purpose": "testing",
	})

	req, w := makeChiRequest(http.MethodPost, "/databases", body, "/databases", nil)
	req = withIdentityRequest(req, productIdentity("frontend"))

	h.Create(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
	require.NotNil(t, capturedDB)
	assert.Equal(t, "frontend", capturedDB.OwnerTeam, "ownerTeam should be auto-filled for product users")
}

func TestCreate_ProductUser_MatchingOwnerTeam(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	body, _ := json.Marshal(map[string]interface{}{
		"name":      "mydb",
		"ownerTeam": "frontend",
	})

	req, w := makeChiRequest(http.MethodPost, "/databases", body, "/databases", nil)
	req = withIdentityRequest(req, productIdentity("frontend"))

	h.Create(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestCreate_ProductUser_CrossTeamRejected(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	body, _ := json.Marshal(map[string]interface{}{
		"name":      "mydb",
		"ownerTeam": "backend",
	})

	req, w := makeChiRequest(http.MethodPost, "/databases", body, "/databases", nil)
	req = withIdentityRequest(req, productIdentity("frontend"))

	h.Create(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "FORBIDDEN", errObj["code"])
	assert.Contains(t, errObj["message"], "Cannot create databases for another team")
}

func TestCreate_PlatformUser_AnyOwnerTeam(t *testing.T) {
	t.Parallel()

	repo := &mockRepo{}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	body, _ := json.Marshal(map[string]interface{}{
		"name":      "mydb",
		"ownerTeam": "any-team",
	})

	req, w := makeChiRequest(http.MethodPost, "/databases", body, "/databases", nil)
	req = withIdentityRequest(req, platformIdentity())

	h.Create(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

// ===== GET /databases — Ownership =====

func TestList_ProductUser_ForcedFilter(t *testing.T) {
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

	// Product user tries to set owner_team query param — should be overridden
	req, w := makeChiRequest(http.MethodGet, "/databases?owner_team=other-team", nil, "/databases", nil)
	req = withIdentityRequest(req, productIdentity("frontend"))

	h.List(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, capturedFilter.OwnerTeam)
	assert.Equal(t, "frontend", *capturedFilter.OwnerTeam, "product user's filter must be forced to own team")
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

	req, w := makeChiRequest(http.MethodGet, "/databases", nil, "/databases", nil)
	req = withIdentityRequest(req, platformIdentity())

	h.List(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Nil(t, capturedFilter.OwnerTeam, "platform user should not have forced filter")
}

// ===== GET /databases/{id} — Ownership =====

func TestGetByID_ProductUser_OwnedDatabase(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	db := sampleDB(id, "provisioning")
	db.OwnerTeam = "frontend"

	repo := &mockRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*database.Database, error) {
			return db, nil
		},
	}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	req, w := makeChiRequest(http.MethodGet, "/databases/"+id.String(), nil, "/databases/{id}", map[string]string{"id": id.String()})
	req = withIdentityRequest(req, productIdentity("frontend"))

	h.GetByID(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestGetByID_ProductUser_NonOwnedReturns404(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	db := sampleDB(id, "provisioning")
	db.OwnerTeam = "backend"

	repo := &mockRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*database.Database, error) {
			return db, nil
		},
	}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	req, w := makeChiRequest(http.MethodGet, "/databases/"+id.String(), nil, "/databases/{id}", map[string]string{"id": id.String()})
	req = withIdentityRequest(req, productIdentity("frontend"))

	h.GetByID(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

func TestGetByID_PlatformUser_SeesAll(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	db := sampleDB(id, "provisioning")
	db.OwnerTeam = "any-team"

	repo := &mockRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*database.Database, error) {
			return db, nil
		},
	}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	req, w := makeChiRequest(http.MethodGet, "/databases/"+id.String(), nil, "/databases/{id}", map[string]string{"id": id.String()})
	req = withIdentityRequest(req, platformIdentity())

	h.GetByID(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// ===== PATCH /databases/{id} — Ownership =====

func TestUpdate_ProductUser_CannotChangeOwnerTeam(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockRepo{}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	body, _ := json.Marshal(map[string]interface{}{
		"ownerTeam": "new-team",
	})
	req, w := makeChiRequest(http.MethodPatch, "/databases/"+id.String(), body, "/databases/{id}", map[string]string{"id": id.String()})
	req = withIdentityRequest(req, productIdentity("frontend"))

	h.Update(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "FORBIDDEN", errObj["code"])
	assert.Contains(t, errObj["message"], "Product users cannot change ownerTeam")
}

func TestUpdate_ProductUser_NonOwnedReturns404(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	db := sampleDB(id, "provisioning")
	db.OwnerTeam = "backend"

	repo := &mockRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*database.Database, error) {
			return db, nil
		},
	}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	body, _ := json.Marshal(map[string]interface{}{
		"purpose": "updated",
	})
	req, w := makeChiRequest(http.MethodPatch, "/databases/"+id.String(), body, "/databases/{id}", map[string]string{"id": id.String()})
	req = withIdentityRequest(req, productIdentity("frontend"))

	h.Update(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

func TestUpdate_ProductUser_OwnedSuccess(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	db := sampleDB(id, "provisioning")
	db.OwnerTeam = "frontend"

	repo := &mockRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*database.Database, error) {
			return db, nil
		},
		updateFn: func(_ context.Context, _ uuid.UUID, fields database.UpdateFields) (*database.Database, error) {
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
	req, w := makeChiRequest(http.MethodPatch, "/databases/"+id.String(), body, "/databases/{id}", map[string]string{"id": id.String()})
	req = withIdentityRequest(req, productIdentity("frontend"))

	h.Update(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestUpdate_PlatformUser_CanChangeOwnerTeam(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	db := sampleDB(id, "provisioning")

	repo := &mockRepo{
		updateFn: func(_ context.Context, _ uuid.UUID, fields database.UpdateFields) (*database.Database, error) {
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
	req, w := makeChiRequest(http.MethodPatch, "/databases/"+id.String(), body, "/databases/{id}", map[string]string{"id": id.String()})
	req = withIdentityRequest(req, platformIdentity())

	h.Update(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	env := parseEnvelope(t, w)
	data := env["data"].(map[string]interface{})
	assert.Equal(t, "new-team", data["ownerTeam"])
}

// ===== DELETE /databases/{id} — Ownership =====

func TestDelete_ProductUser_OwnedSuccess(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	db := sampleDB(id, "provisioning")
	db.OwnerTeam = "frontend"

	repo := &mockRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*database.Database, error) {
			return db, nil
		},
	}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	req, w := makeChiRequest(http.MethodDelete, "/databases/"+id.String(), nil, "/databases/{id}", map[string]string{"id": id.String()})
	req = withIdentityRequest(req, productIdentity("frontend"))

	h.Delete(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestDelete_ProductUser_NonOwnedReturns404(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	db := sampleDB(id, "provisioning")
	db.OwnerTeam = "backend"

	repo := &mockRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*database.Database, error) {
			return db, nil
		},
	}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	req, w := makeChiRequest(http.MethodDelete, "/databases/"+id.String(), nil, "/databases/{id}", map[string]string{"id": id.String()})
	req = withIdentityRequest(req, productIdentity("frontend"))

	h.Delete(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

func TestDelete_PlatformUser_AnyDatabase(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	db := sampleDB(id, "provisioning")
	db.OwnerTeam = "other-team"

	repo := &mockRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*database.Database, error) {
			return db, nil
		},
	}
	mgr := &mockManager{}
	h := newTestHandler(repo, mgr)

	req, w := makeChiRequest(http.MethodDelete, "/databases/"+id.String(), nil, "/databases/{id}", map[string]string{"id": id.String()})
	req = withIdentityRequest(req, platformIdentity())

	h.Delete(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}
