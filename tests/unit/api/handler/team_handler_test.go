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
	"github.com/daap14/daap/internal/team"
)

// --- Mock Team Repository ---

type mockTeamRepo struct {
	createFn  func(ctx context.Context, t *team.Team) error
	getByIDFn func(ctx context.Context, id uuid.UUID) (*team.Team, error)
	listFn    func(ctx context.Context) ([]team.Team, error)
	deleteFn  func(ctx context.Context, id uuid.UUID) error
}

func (m *mockTeamRepo) Create(ctx context.Context, t *team.Team) error {
	if m.createFn != nil {
		return m.createFn(ctx, t)
	}
	t.ID = uuid.New()
	t.CreatedAt = time.Now().UTC()
	t.UpdatedAt = time.Now().UTC()
	return nil
}

func (m *mockTeamRepo) GetByID(ctx context.Context, id uuid.UUID) (*team.Team, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, team.ErrTeamNotFound
}

func (m *mockTeamRepo) List(ctx context.Context) ([]team.Team, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return []team.Team{}, nil
}

func (m *mockTeamRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

// --- Helpers ---

func newTeamHandler(repo team.Repository) *handler.TeamHandler {
	return handler.NewTeamHandler(repo)
}

func sampleTeam(id uuid.UUID) *team.Team {
	now := time.Now().UTC()
	return &team.Team{
		ID:        id,
		Name:      "ops",
		Role:      "platform",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// ===== POST /teams =====

func TestTeamCreate_Success(t *testing.T) {
	t.Parallel()

	repo := &mockTeamRepo{}
	h := newTeamHandler(repo)

	body, _ := json.Marshal(map[string]interface{}{
		"name": "ops",
		"role": "platform",
	})

	req, w := makeChiRequest(http.MethodPost, "/teams", body, "/teams", nil)

	h.Create(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	env := parseEnvelope(t, w)
	assert.Nil(t, env["error"])
	data := env["data"].(map[string]interface{})
	assert.Equal(t, "ops", data["name"])
	assert.Equal(t, "platform", data["role"])
	assert.NotEmpty(t, data["id"])
	assert.NotEmpty(t, data["createdAt"])
	assert.NotEmpty(t, data["updatedAt"])
}

func TestTeamCreate_ValidationError_MissingFields(t *testing.T) {
	t.Parallel()

	repo := &mockTeamRepo{}
	h := newTeamHandler(repo)

	body, _ := json.Marshal(map[string]interface{}{})

	req, w := makeChiRequest(http.MethodPost, "/teams", body, "/teams", nil)

	h.Create(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])
	details := errObj["details"].([]interface{})
	assert.Len(t, details, 2) // name + role
}

func TestTeamCreate_ValidationError_InvalidRole(t *testing.T) {
	t.Parallel()

	repo := &mockTeamRepo{}
	h := newTeamHandler(repo)

	body, _ := json.Marshal(map[string]interface{}{
		"name": "ops",
		"role": "admin",
	})

	req, w := makeChiRequest(http.MethodPost, "/teams", body, "/teams", nil)

	h.Create(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])
}

func TestTeamCreate_DuplicateName(t *testing.T) {
	t.Parallel()

	repo := &mockTeamRepo{
		createFn: func(_ context.Context, _ *team.Team) error {
			return team.ErrDuplicateTeamName
		},
	}
	h := newTeamHandler(repo)

	body, _ := json.Marshal(map[string]interface{}{
		"name": "ops",
		"role": "platform",
	})

	req, w := makeChiRequest(http.MethodPost, "/teams", body, "/teams", nil)

	h.Create(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "DUPLICATE_NAME", errObj["code"])
}

func TestTeamCreate_InvalidJSON(t *testing.T) {
	t.Parallel()

	repo := &mockTeamRepo{}
	h := newTeamHandler(repo)

	req, w := makeChiRequest(http.MethodPost, "/teams", []byte("{invalid"), "/teams", nil)

	h.Create(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_JSON", errObj["code"])
}

// ===== GET /teams =====

func TestTeamList_Empty(t *testing.T) {
	t.Parallel()

	repo := &mockTeamRepo{}
	h := newTeamHandler(repo)

	req, w := makeChiRequest(http.MethodGet, "/teams", nil, "/teams", nil)

	h.List(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err)

	data := env["data"].([]interface{})
	assert.Len(t, data, 0)

	meta := env["meta"].(map[string]interface{})
	assert.Equal(t, float64(0), meta["total"])
	assert.Equal(t, float64(1), meta["page"])
	assert.Equal(t, float64(100), meta["limit"])
}

func TestTeamList_NonEmpty(t *testing.T) {
	t.Parallel()

	id1 := uuid.New()
	id2 := uuid.New()
	repo := &mockTeamRepo{
		listFn: func(_ context.Context) ([]team.Team, error) {
			return []team.Team{
				*sampleTeam(id1),
				{
					ID:        id2,
					Name:      "frontend",
					Role:      "product",
					CreatedAt: time.Now().UTC(),
					UpdatedAt: time.Now().UTC(),
				},
			}, nil
		},
	}
	h := newTeamHandler(repo)

	req, w := makeChiRequest(http.MethodGet, "/teams", nil, "/teams", nil)

	h.List(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err)

	data := env["data"].([]interface{})
	assert.Len(t, data, 2)

	meta := env["meta"].(map[string]interface{})
	assert.Equal(t, float64(2), meta["total"])

	first := data[0].(map[string]interface{})
	assert.Equal(t, "ops", first["name"])
	assert.Equal(t, "platform", first["role"])
}

// ===== DELETE /teams/{id} =====

func TestTeamDelete_Success(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockTeamRepo{
		deleteFn: func(_ context.Context, reqID uuid.UUID) error {
			assert.Equal(t, id, reqID)
			return nil
		},
	}
	h := newTeamHandler(repo)

	req, w := makeChiRequest(http.MethodDelete, "/teams/"+id.String(), nil, "/teams/{id}", map[string]string{"id": id.String()})

	h.Delete(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestTeamDelete_NotFound(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockTeamRepo{
		deleteFn: func(_ context.Context, _ uuid.UUID) error {
			return team.ErrTeamNotFound
		},
	}
	h := newTeamHandler(repo)

	req, w := makeChiRequest(http.MethodDelete, "/teams/"+id.String(), nil, "/teams/{id}", map[string]string{"id": id.String()})

	h.Delete(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

func TestTeamDelete_TeamHasUsers(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	repo := &mockTeamRepo{
		deleteFn: func(_ context.Context, _ uuid.UUID) error {
			return team.ErrTeamHasUsers
		},
	}
	h := newTeamHandler(repo)

	req, w := makeChiRequest(http.MethodDelete, "/teams/"+id.String(), nil, "/teams/{id}", map[string]string{"id": id.String()})

	h.Delete(w, req)

	assert.Equal(t, http.StatusConflict, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "TEAM_HAS_USERS", errObj["code"])
}

func TestTeamDelete_InvalidID(t *testing.T) {
	t.Parallel()

	repo := &mockTeamRepo{}
	h := newTeamHandler(repo)

	req, w := makeChiRequest(http.MethodDelete, "/teams/not-a-uuid", nil, "/teams/{id}", map[string]string{"id": "not-a-uuid"})

	h.Delete(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_ID", errObj["code"])
}
