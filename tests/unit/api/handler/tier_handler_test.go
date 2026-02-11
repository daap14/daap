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
	"github.com/daap14/daap/internal/blueprint"
	"github.com/daap14/daap/internal/tier"
)

// --- Helpers ---

func newTierHandler(repo tier.Repository) *handler.TierHandler {
	bpRepo := &mockBlueprintRepo{}
	return handler.NewTierHandler(repo, bpRepo)
}

func newTierHandlerWithBP(repo tier.Repository, bpRepo blueprint.Repository) *handler.TierHandler {
	return handler.NewTierHandler(repo, bpRepo)
}

func sampleTier(id uuid.UUID) *tier.Tier {
	now := time.Now().UTC()
	bpID := uuid.New()
	return &tier.Tier{
		ID:                  id,
		Name:                "standard",
		Description:         "Standard tier",
		BlueprintID:         &bpID,
		BlueprintName:       "cnpg-standard",
		DestructionStrategy: "hard_delete",
		BackupEnabled:       false,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
}

// --- Mock Blueprint Repository ---

type mockBlueprintRepo struct {
	getByNameFn func(ctx context.Context, name string) (*blueprint.Blueprint, error)
	getByIDFn   func(ctx context.Context, id uuid.UUID) (*blueprint.Blueprint, error)
	createFn    func(ctx context.Context, bp *blueprint.Blueprint) error
	listFn      func(ctx context.Context) ([]blueprint.Blueprint, error)
	deleteFn    func(ctx context.Context, id uuid.UUID) error
}

func (m *mockBlueprintRepo) Create(ctx context.Context, bp *blueprint.Blueprint) error {
	if m.createFn != nil {
		return m.createFn(ctx, bp)
	}
	return nil
}

func (m *mockBlueprintRepo) GetByID(ctx context.Context, id uuid.UUID) (*blueprint.Blueprint, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, blueprint.ErrBlueprintNotFound
}

func (m *mockBlueprintRepo) GetByName(ctx context.Context, name string) (*blueprint.Blueprint, error) {
	if m.getByNameFn != nil {
		return m.getByNameFn(ctx, name)
	}
	return &blueprint.Blueprint{
		ID:       uuid.New(),
		Name:     name,
		Provider: "cnpg",
	}, nil
}

func (m *mockBlueprintRepo) List(ctx context.Context) ([]blueprint.Blueprint, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return []blueprint.Blueprint{}, nil
}

func (m *mockBlueprintRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
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
		"blueprintName":       "cnpg-standard",
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

func TestTierCreate_WithBlueprint(t *testing.T) {
	t.Parallel()

	bpID := uuid.New()
	bpRepo := &mockBlueprintRepo{
		getByNameFn: func(_ context.Context, name string) (*blueprint.Blueprint, error) {
			return &blueprint.Blueprint{ID: bpID, Name: name, Provider: "cnpg"}, nil
		},
	}
	repo := &mockTierRepo{
		createFn: func(_ context.Context, t *tier.Tier) error {
			t.ID = uuid.New()
			t.CreatedAt = time.Now().UTC()
			t.UpdatedAt = time.Now().UTC()
			return nil
		},
	}
	h := newTierHandlerWithBP(repo, bpRepo)

	body, _ := json.Marshal(map[string]interface{}{
		"name":                "standard",
		"blueprintName":       "cnpg-standard",
		"destructionStrategy": "hard_delete",
	})

	req, w := makeChiRequest(http.MethodPost, "/tiers", body, "/tiers", nil)
	h.Create(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestTierCreate_BlueprintNotFound(t *testing.T) {
	t.Parallel()

	bpRepo := &mockBlueprintRepo{
		getByNameFn: func(_ context.Context, _ string) (*blueprint.Blueprint, error) {
			return nil, blueprint.ErrBlueprintNotFound
		},
	}
	repo := &mockTierRepo{}
	h := newTierHandlerWithBP(repo, bpRepo)

	body, _ := json.Marshal(map[string]interface{}{
		"name":                "standard",
		"blueprintName":       "nonexistent",
		"destructionStrategy": "hard_delete",
	})

	req, w := makeChiRequest(http.MethodPost, "/tiers", body, "/tiers", nil)
	h.Create(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

func TestTierCreate_ValidationError(t *testing.T) {
	t.Parallel()

	repo := &mockTierRepo{}
	h := newTierHandler(repo)

	body, _ := json.Marshal(map[string]interface{}{
		"name": "",
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
		"blueprintName":       "cnpg-standard",
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
	assert.NotNil(t, first["destructionStrategy"], "platform user should see destructionStrategy")
	assert.NotNil(t, first["blueprintName"], "platform user should see blueprintName")
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
	assert.Nil(t, first["destructionStrategy"], "product user should NOT see destructionStrategy")
	assert.Nil(t, first["blueprintId"], "product user should NOT see blueprintId")
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
	assert.Nil(t, data["destructionStrategy"], "product user should NOT see destructionStrategy")
	assert.Nil(t, data["blueprintId"], "product user should NOT see blueprintId")
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
