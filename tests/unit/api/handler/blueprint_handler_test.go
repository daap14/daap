package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/daap14/daap/internal/api/handler"
	"github.com/daap14/daap/internal/blueprint"
	"github.com/daap14/daap/internal/provider"
)

// --- Helpers ---

func testRegistry() *provider.Registry {
	reg := provider.NewRegistry()
	reg.Register("cnpg", nil)
	return reg
}

func newBlueprintHandler(repo blueprint.Repository) *handler.BlueprintHandler {
	return handler.NewBlueprintHandler(repo, testRegistry())
}

func sampleBlueprint(id uuid.UUID) *blueprint.Blueprint {
	now := time.Now().UTC()
	return &blueprint.Blueprint{
		ID:        id,
		Name:      "cnpg-standard",
		Provider:  "cnpg",
		Manifests: "apiVersion: postgresql.cnpg.io/v1\nkind: Cluster\nmetadata:\n  name: test\nspec:\n  instances: 1",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

const validManifests = `apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: test-cluster
spec:
  instances: 1`

// ===== POST /blueprints =====

func TestBlueprintCreate_Success(t *testing.T) {
	t.Parallel()

	repo := &mockBlueprintRepo{}
	h := newBlueprintHandler(repo)

	body, _ := json.Marshal(map[string]interface{}{
		"name":      "cnpg-standard",
		"provider":  "cnpg",
		"manifests": validManifests,
	})

	req, w := makeChiRequest(http.MethodPost, "/blueprints", body, "", nil)
	h.Create(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	env := parseEnvelope(t, w)
	data := env["data"].(map[string]interface{})
	assert.NotEmpty(t, data["id"])
	assert.Equal(t, "cnpg-standard", data["name"])
	assert.Equal(t, "cnpg", data["provider"])
	assert.NotEmpty(t, data["createdAt"])
}

func TestBlueprintCreate_MissingName(t *testing.T) {
	t.Parallel()

	repo := &mockBlueprintRepo{}
	h := newBlueprintHandler(repo)

	body, _ := json.Marshal(map[string]interface{}{
		"provider":  "cnpg",
		"manifests": validManifests,
	})

	req, w := makeChiRequest(http.MethodPost, "/blueprints", body, "", nil)
	h.Create(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])
}

func TestBlueprintCreate_DuplicateName(t *testing.T) {
	t.Parallel()

	repo := &mockBlueprintRepo{
		createFn: func(_ context.Context, _ *blueprint.Blueprint) error {
			return blueprint.ErrDuplicateBlueprintName
		},
	}
	h := newBlueprintHandler(repo)

	body, _ := json.Marshal(map[string]interface{}{
		"name":      "cnpg-standard",
		"provider":  "cnpg",
		"manifests": validManifests,
	})

	req, w := makeChiRequest(http.MethodPost, "/blueprints", body, "", nil)
	h.Create(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "DUPLICATE_NAME", errObj["code"])
}

func TestBlueprintCreate_UnknownProvider(t *testing.T) {
	t.Parallel()

	repo := &mockBlueprintRepo{}
	h := newBlueprintHandler(repo)

	body, _ := json.Marshal(map[string]interface{}{
		"name":      "test-bp",
		"provider":  "unknown",
		"manifests": validManifests,
	})

	req, w := makeChiRequest(http.MethodPost, "/blueprints", body, "", nil)
	h.Create(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])
}

func TestBlueprintCreate_InvalidJSON(t *testing.T) {
	t.Parallel()

	repo := &mockBlueprintRepo{}
	h := newBlueprintHandler(repo)

	req, w := makeChiRequest(http.MethodPost, "/blueprints", []byte("not json"), "", nil)
	h.Create(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_JSON", errObj["code"])
}

// ===== GET /blueprints =====

func TestBlueprintList_Empty(t *testing.T) {
	t.Parallel()

	repo := &mockBlueprintRepo{}
	h := newBlueprintHandler(repo)

	req, w := makeChiRequest(http.MethodGet, "/blueprints", nil, "", nil)
	h.List(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	env := parseEnvelope(t, w)
	data := env["data"].([]interface{})
	assert.Len(t, data, 0)
}

func TestBlueprintList_WithResults(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	repo := &mockBlueprintRepo{
		listFn: func(_ context.Context) ([]blueprint.Blueprint, error) {
			return []blueprint.Blueprint{
				{ID: uuid.New(), Name: "bp-one", Provider: "cnpg", Manifests: "m1", CreatedAt: now, UpdatedAt: now},
				{ID: uuid.New(), Name: "bp-two", Provider: "cnpg", Manifests: "m2", CreatedAt: now, UpdatedAt: now},
			}, nil
		},
	}
	h := newBlueprintHandler(repo)

	req, w := makeChiRequest(http.MethodGet, "/blueprints", nil, "", nil)
	h.List(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	env := parseEnvelope(t, w)
	data := env["data"].([]interface{})
	assert.Len(t, data, 2)
}

// ===== GET /blueprints/{id} =====

func TestBlueprintGetByID_Success(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockBlueprintRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*blueprint.Blueprint, error) {
			return sampleBlueprint(id), nil
		},
	}
	h := newBlueprintHandler(repo)

	req, w := makeChiRequest(http.MethodGet, "/blueprints/"+id.String(), nil, "/blueprints/{id}", map[string]string{"id": id.String()})
	h.GetByID(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	env := parseEnvelope(t, w)
	data := env["data"].(map[string]interface{})
	assert.Equal(t, id.String(), data["id"])
	assert.Equal(t, "cnpg-standard", data["name"])
}

func TestBlueprintGetByID_NotFound(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockBlueprintRepo{}
	h := newBlueprintHandler(repo)

	req, w := makeChiRequest(http.MethodGet, "/blueprints/"+id.String(), nil, "/blueprints/{id}", map[string]string{"id": id.String()})
	h.GetByID(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

func TestBlueprintGetByID_InvalidUUID(t *testing.T) {
	t.Parallel()

	repo := &mockBlueprintRepo{}
	h := newBlueprintHandler(repo)

	req, w := makeChiRequest(http.MethodGet, "/blueprints/not-a-uuid", nil, "/blueprints/{id}", map[string]string{"id": "not-a-uuid"})
	h.GetByID(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_ID", errObj["code"])
}

// ===== DELETE /blueprints/{id} =====

func TestBlueprintDelete_Success(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockBlueprintRepo{}
	h := newBlueprintHandler(repo)

	req, w := makeChiRequest(http.MethodDelete, "/blueprints/"+id.String(), nil, "/blueprints/{id}", map[string]string{"id": id.String()})
	h.Delete(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestBlueprintDelete_NotFound(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockBlueprintRepo{
		deleteFn: func(_ context.Context, _ uuid.UUID) error {
			return blueprint.ErrBlueprintNotFound
		},
	}
	h := newBlueprintHandler(repo)

	req, w := makeChiRequest(http.MethodDelete, "/blueprints/"+id.String(), nil, "/blueprints/{id}", map[string]string{"id": id.String()})
	h.Delete(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

func TestBlueprintDelete_HasTiers(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockBlueprintRepo{
		deleteFn: func(_ context.Context, _ uuid.UUID) error {
			return blueprint.ErrBlueprintHasTiers
		},
	}
	h := newBlueprintHandler(repo)

	req, w := makeChiRequest(http.MethodDelete, "/blueprints/"+id.String(), nil, "/blueprints/{id}", map[string]string{"id": id.String()})
	h.Delete(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "BLUEPRINT_HAS_TIERS", errObj["code"])
}

func TestBlueprintDelete_InvalidUUID(t *testing.T) {
	t.Parallel()

	repo := &mockBlueprintRepo{}
	h := newBlueprintHandler(repo)

	req, w := makeChiRequest(http.MethodDelete, "/blueprints/bad-uuid", nil, "/blueprints/{id}", map[string]string{"id": "bad-uuid"})
	h.Delete(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_ID", errObj["code"])
}
