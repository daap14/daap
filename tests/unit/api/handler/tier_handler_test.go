package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daap14/daap/internal/api/handler"
	"github.com/daap14/daap/internal/tier"
)

// --- Helpers ---

func newTierHandler(repo tier.Repository) *handler.TierHandler {
	return handler.NewTierHandler(repo)
}

func sampleTier(id uuid.UUID) *tier.Tier {
	now := time.Now().UTC()
	return &tier.Tier{
		ID:                  id,
		Name:                "standard",
		Description:         "Standard tier",
		Instances:           1,
		CPU:                 "500m",
		Memory:              "512Mi",
		StorageSize:         "1Gi",
		StorageClass:        "gp3",
		PGVersion:           "16",
		PoolMode:            "transaction",
		MaxConnections:      100,
		DestructionStrategy: "hard_delete",
		BackupEnabled:       false,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
}

// ===== POST /tiers =====

func TestTierCreate_Success(t *testing.T) {
	t.Parallel()

	repo := &mockTierRepo{
		getByNameFn: func(_ context.Context, _ string) (*tier.Tier, error) {
			return nil, tier.ErrTierNotFound
		},
		createFn: func(_ context.Context, t *tier.Tier) error {
			t.ID = uuid.New()
			t.CreatedAt = time.Now().UTC()
			t.UpdatedAt = time.Now().UTC()
			return nil
		},
	}
	h := newTierHandler(repo)

	body, _ := json.Marshal(map[string]interface{}{
		"name":                "standard",
		"description":         "Standard tier",
		"instances":           1,
		"cpu":                 "500m",
		"memory":              "512Mi",
		"storageSize":         "1Gi",
		"storageClass":        "gp3",
		"pgVersion":           "16",
		"poolMode":            "transaction",
		"maxConnections":      100,
		"destructionStrategy": "hard_delete",
		"backupEnabled":       false,
	})

	req, w := makeChiRequest(http.MethodPost, "/tiers", body, "/tiers", nil)
	h.Create(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	env := parseEnvelope(t, w)
	assert.Nil(t, env["error"])
	data := env["data"].(map[string]interface{})
	assert.Equal(t, "standard", data["name"])
	assert.NotEmpty(t, data["id"])
	assert.NotEmpty(t, data["createdAt"])
}

func TestTierCreate_ValidationError(t *testing.T) {
	t.Parallel()

	repo := &mockTierRepo{}
	h := newTierHandler(repo)

	body, _ := json.Marshal(map[string]interface{}{
		"name":           "",
		"instances":      0,
		"cpu":            "",
		"memory":         "",
		"storageSize":    "",
		"pgVersion":      "",
		"poolMode":       "",
		"maxConnections": 0,
	})

	req, w := makeChiRequest(http.MethodPost, "/tiers", body, "/tiers", nil)
	h.Create(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])
	assert.NotNil(t, errObj["details"])
}

func TestTierCreate_DuplicateName(t *testing.T) {
	t.Parallel()

	repo := &mockTierRepo{
		createFn: func(_ context.Context, _ *tier.Tier) error {
			return tier.ErrDuplicateTierName
		},
	}
	h := newTierHandler(repo)

	body, _ := json.Marshal(map[string]interface{}{
		"name":                "standard",
		"instances":           1,
		"cpu":                 "500m",
		"memory":              "512Mi",
		"storageSize":         "1Gi",
		"pgVersion":           "16",
		"poolMode":            "transaction",
		"maxConnections":      100,
		"destructionStrategy": "hard_delete",
	})

	req, w := makeChiRequest(http.MethodPost, "/tiers", body, "/tiers", nil)
	h.Create(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "DUPLICATE_NAME", errObj["code"])
}

func TestTierCreate_InvalidJSON(t *testing.T) {
	t.Parallel()

	repo := &mockTierRepo{}
	h := newTierHandler(repo)

	req, w := makeChiRequest(http.MethodPost, "/tiers", []byte("{invalid"), "/tiers", nil)
	h.Create(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_JSON", errObj["code"])
}

// ===== GET /tiers =====

func TestTierList_PlatformUser_FullResponse(t *testing.T) {
	t.Parallel()

	id1 := uuid.New()
	id2 := uuid.New()
	repo := &mockTierRepo{
		listFn: func(_ context.Context) ([]tier.Tier, error) {
			return []tier.Tier{*sampleTier(id1), *sampleTier(id2)}, nil
		},
	}
	h := newTierHandler(repo)

	identity := platformIdentity()
	req, w := makeAuthRequest(http.MethodGet, "/tiers", nil, nil, identity)
	h.List(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err)

	data := env["data"].([]interface{})
	assert.Len(t, data, 2)

	first := data[0].(map[string]interface{})
	assert.NotNil(t, first["cpu"], "platform user should see cpu")
	assert.NotNil(t, first["memory"], "platform user should see memory")
	assert.NotNil(t, first["instances"], "platform user should see instances")
}

func TestTierList_ProductUser_RedactedResponse(t *testing.T) {
	t.Parallel()

	id1 := uuid.New()
	myTeamID := uuid.New()
	repo := &mockTierRepo{
		listFn: func(_ context.Context) ([]tier.Tier, error) {
			return []tier.Tier{*sampleTier(id1)}, nil
		},
	}
	h := newTierHandler(repo)

	identity := productIdentity("my-team", myTeamID)
	req, w := makeAuthRequest(http.MethodGet, "/tiers", nil, nil, identity)
	h.List(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err)

	data := env["data"].([]interface{})
	assert.Len(t, data, 1)

	first := data[0].(map[string]interface{})
	assert.NotNil(t, first["id"], "product user should see id")
	assert.NotNil(t, first["name"], "product user should see name")
	assert.NotNil(t, first["description"], "product user should see description")
	assert.Nil(t, first["cpu"], "product user should NOT see cpu")
	assert.Nil(t, first["memory"], "product user should NOT see memory")
	assert.Nil(t, first["instances"], "product user should NOT see instances")
}

func TestTierList_Empty(t *testing.T) {
	t.Parallel()

	repo := &mockTierRepo{
		listFn: func(_ context.Context) ([]tier.Tier, error) {
			return []tier.Tier{}, nil
		},
	}
	h := newTierHandler(repo)

	req, w := makeChiRequest(http.MethodGet, "/tiers", nil, "/tiers", nil)
	h.List(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err)

	data := env["data"].([]interface{})
	assert.Len(t, data, 0)
}

// ===== GET /tiers/{id} =====

func TestTierGetByID_Success(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockTierRepo{
		getByIDFn: func(_ context.Context, reqID uuid.UUID) (*tier.Tier, error) {
			assert.Equal(t, id, reqID)
			return sampleTier(id), nil
		},
	}
	h := newTierHandler(repo)

	req, w := makeChiRequest(http.MethodGet, "/tiers/"+id.String(), nil, "/tiers/{id}", map[string]string{"id": id.String()})
	h.GetByID(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	env := parseEnvelope(t, w)
	assert.Nil(t, env["error"])
	data := env["data"].(map[string]interface{})
	assert.Equal(t, id.String(), data["id"])
	assert.Equal(t, "standard", data["name"])
}

func TestTierGetByID_NotFound(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockTierRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*tier.Tier, error) {
			return nil, tier.ErrTierNotFound
		},
	}
	h := newTierHandler(repo)

	req, w := makeChiRequest(http.MethodGet, "/tiers/"+id.String(), nil, "/tiers/{id}", map[string]string{"id": id.String()})
	h.GetByID(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

func TestTierGetByID_InvalidUUID(t *testing.T) {
	t.Parallel()

	repo := &mockTierRepo{}
	h := newTierHandler(repo)

	req, w := makeChiRequest(http.MethodGet, "/tiers/not-a-uuid", nil, "/tiers/{id}", map[string]string{"id": "not-a-uuid"})
	h.GetByID(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_ID", errObj["code"])
}

func TestTierGetByID_ProductUser_Redacted(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	myTeamID := uuid.New()
	repo := &mockTierRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*tier.Tier, error) {
			return sampleTier(id), nil
		},
	}
	h := newTierHandler(repo)

	identity := productIdentity("my-team", myTeamID)
	req, w := makeAuthRequest(http.MethodGet, "/tiers/"+id.String(), nil, map[string]string{"id": id.String()}, identity)
	h.GetByID(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	env := parseEnvelope(t, w)
	data := env["data"].(map[string]interface{})
	assert.NotNil(t, data["id"])
	assert.NotNil(t, data["name"])
	assert.NotNil(t, data["description"])
	assert.Nil(t, data["cpu"], "product user should NOT see cpu")
	assert.Nil(t, data["memory"], "product user should NOT see memory")
}

// ===== PATCH /tiers/{id} =====

func TestTierUpdate_Success(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockTierRepo{
		updateFn: func(_ context.Context, reqID uuid.UUID, fields tier.UpdateFields) (*tier.Tier, error) {
			assert.Equal(t, id, reqID)
			t2 := sampleTier(id)
			if fields.Description != nil {
				t2.Description = *fields.Description
			}
			return t2, nil
		},
	}
	h := newTierHandler(repo)

	body, _ := json.Marshal(map[string]interface{}{
		"description": "Updated description",
		"instances":   3,
	})

	req, w := makeChiRequest(http.MethodPatch, "/tiers/"+id.String(), body, "/tiers/{id}", map[string]string{"id": id.String()})
	h.Update(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	env := parseEnvelope(t, w)
	assert.Nil(t, env["error"])
	data := env["data"].(map[string]interface{})
	assert.Equal(t, id.String(), data["id"])
}

func TestTierUpdate_ImmutableName(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockTierRepo{}
	h := newTierHandler(repo)

	body, _ := json.Marshal(map[string]interface{}{
		"name": "new-name",
	})

	req, w := makeChiRequest(http.MethodPatch, "/tiers/"+id.String(), body, "/tiers/{id}", map[string]string{"id": id.String()})
	h.Update(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "IMMUTABLE_FIELD", errObj["code"])
}

func TestTierUpdate_NotFound(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockTierRepo{
		updateFn: func(_ context.Context, _ uuid.UUID, _ tier.UpdateFields) (*tier.Tier, error) {
			return nil, tier.ErrTierNotFound
		},
	}
	h := newTierHandler(repo)

	body, _ := json.Marshal(map[string]interface{}{
		"description": "Updated",
	})

	req, w := makeChiRequest(http.MethodPatch, "/tiers/"+id.String(), body, "/tiers/{id}", map[string]string{"id": id.String()})
	h.Update(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

func TestTierUpdate_InvalidJSON(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockTierRepo{}
	h := newTierHandler(repo)

	req, w := makeChiRequest(http.MethodPatch, "/tiers/"+id.String(), []byte("{invalid"), "/tiers/{id}", map[string]string{"id": id.String()})
	h.Update(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_JSON", errObj["code"])
}

func TestTierUpdate_InvalidUUID(t *testing.T) {
	t.Parallel()

	repo := &mockTierRepo{}
	h := newTierHandler(repo)

	body, _ := json.Marshal(map[string]interface{}{
		"description": "Updated",
	})

	req, w := makeChiRequest(http.MethodPatch, "/tiers/not-a-uuid", body, "/tiers/{id}", map[string]string{"id": "not-a-uuid"})
	h.Update(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_ID", errObj["code"])
}

// ===== DELETE /tiers/{id} =====

func TestTierDelete_Success(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockTierRepo{
		deleteFn: func(_ context.Context, reqID uuid.UUID) error {
			assert.Equal(t, id, reqID)
			return nil
		},
	}
	h := newTierHandler(repo)

	req, w := makeChiRequest(http.MethodDelete, "/tiers/"+id.String(), nil, "/tiers/{id}", map[string]string{"id": id.String()})
	h.Delete(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestTierDelete_NotFound(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockTierRepo{
		deleteFn: func(_ context.Context, _ uuid.UUID) error {
			return tier.ErrTierNotFound
		},
	}
	h := newTierHandler(repo)

	req, w := makeChiRequest(http.MethodDelete, "/tiers/"+id.String(), nil, "/tiers/{id}", map[string]string{"id": id.String()})
	h.Delete(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

func TestTierDelete_HasDatabases(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockTierRepo{
		deleteFn: func(_ context.Context, _ uuid.UUID) error {
			return tier.ErrTierHasDatabases
		},
	}
	h := newTierHandler(repo)

	req, w := makeChiRequest(http.MethodDelete, "/tiers/"+id.String(), nil, "/tiers/{id}", map[string]string{"id": id.String()})
	h.Delete(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "TIER_HAS_DATABASES", errObj["code"])
}

func TestTierDelete_InvalidUUID(t *testing.T) {
	t.Parallel()

	repo := &mockTierRepo{}
	h := newTierHandler(repo)

	req, w := makeChiRequest(http.MethodDelete, "/tiers/not-a-uuid", nil, "/tiers/{id}", map[string]string{"id": "not-a-uuid"})
	h.Delete(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_ID", errObj["code"])
}
