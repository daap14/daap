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
	"github.com/daap14/daap/internal/auth"
	"github.com/daap14/daap/internal/team"
)

// --- Mock User Repository ---

type mockUserRepo struct {
	createFn     func(ctx context.Context, u *auth.User) error
	getByIDFn    func(ctx context.Context, id uuid.UUID) (*auth.User, error)
	findByPrefixFn func(ctx context.Context, prefix string) ([]auth.User, error)
	listFn       func(ctx context.Context) ([]auth.User, error)
	revokeFn     func(ctx context.Context, id uuid.UUID) error
	countAllFn   func(ctx context.Context) (int, error)
}

func (m *mockUserRepo) Create(ctx context.Context, u *auth.User) error {
	if m.createFn != nil {
		return m.createFn(ctx, u)
	}
	u.ID = uuid.New()
	u.CreatedAt = time.Now().UTC()
	return nil
}

func (m *mockUserRepo) GetByID(ctx context.Context, id uuid.UUID) (*auth.User, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, id)
	}
	return nil, auth.ErrUserNotFound
}

func (m *mockUserRepo) FindByPrefix(ctx context.Context, prefix string) ([]auth.User, error) {
	if m.findByPrefixFn != nil {
		return m.findByPrefixFn(ctx, prefix)
	}
	return []auth.User{}, nil
}

func (m *mockUserRepo) List(ctx context.Context) ([]auth.User, error) {
	if m.listFn != nil {
		return m.listFn(ctx)
	}
	return []auth.User{}, nil
}

func (m *mockUserRepo) Revoke(ctx context.Context, id uuid.UUID) error {
	if m.revokeFn != nil {
		return m.revokeFn(ctx, id)
	}
	return nil
}

func (m *mockUserRepo) CountAll(ctx context.Context) (int, error) {
	if m.countAllFn != nil {
		return m.countAllFn(ctx)
	}
	return 0, nil
}

// --- Helpers ---

func newUserHandler(authSvc *auth.Service, userRepo auth.UserRepository, teamRepo team.Repository) *handler.UserHandler {
	return handler.NewUserHandler(authSvc, userRepo, teamRepo)
}

func sampleUser(id uuid.UUID, teamID *uuid.UUID, isSuperuser bool) *auth.User {
	now := time.Now().UTC()
	return &auth.User{
		ID:           id,
		Name:         "alice",
		TeamID:       teamID,
		IsSuperuser:  isSuperuser,
		ApiKeyPrefix: "daap_abc",
		ApiKeyHash:   "$2a$12$fakehash",
		CreatedAt:    now,
	}
}

// ===== POST /users =====

func TestUserCreate_Success(t *testing.T) {
	t.Parallel()

	teamID := uuid.New()
	teamRepo := &mockTeamRepo{
		getByIDFn: func(_ context.Context, id uuid.UUID) (*team.Team, error) {
			assert.Equal(t, teamID, id)
			return &team.Team{
				ID:        teamID,
				Name:      "ops",
				Role:      "platform",
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}, nil
		},
	}
	userRepo := &mockUserRepo{}

	// Use a real auth.Service with low bcrypt cost for speed
	authSvc := auth.NewService(userRepo, teamRepo, 4)
	h := newUserHandler(authSvc, userRepo, teamRepo)

	body, _ := json.Marshal(map[string]interface{}{
		"name":   "alice",
		"teamId": teamID.String(),
	})

	req, w := makeChiRequest(http.MethodPost, "/users", body, "/users", nil)

	h.Create(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	env := parseEnvelope(t, w)
	assert.Nil(t, env["error"])
	data := env["data"].(map[string]interface{})
	assert.Equal(t, "alice", data["name"])
	assert.Equal(t, teamID.String(), data["teamId"])
	assert.Equal(t, "ops", data["teamName"])
	assert.Equal(t, "platform", data["role"])
	assert.NotEmpty(t, data["apiKey"])
	assert.NotEmpty(t, data["id"])
	assert.NotEmpty(t, data["createdAt"])

	// apiKey should start with "daap_"
	apiKey := data["apiKey"].(string)
	assert.True(t, len(apiKey) > 8, "apiKey should be longer than 8 chars")
	assert.Equal(t, "daap_", apiKey[:5])
}

func TestUserCreate_ValidationError_MissingFields(t *testing.T) {
	t.Parallel()

	teamRepo := &mockTeamRepo{}
	userRepo := &mockUserRepo{}
	authSvc := auth.NewService(userRepo, teamRepo, 4)
	h := newUserHandler(authSvc, userRepo, teamRepo)

	body, _ := json.Marshal(map[string]interface{}{})

	req, w := makeChiRequest(http.MethodPost, "/users", body, "/users", nil)

	h.Create(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])
	details := errObj["details"].([]interface{})
	assert.Len(t, details, 2) // name + teamId
}

func TestUserCreate_ValidationError_InvalidTeamID(t *testing.T) {
	t.Parallel()

	teamRepo := &mockTeamRepo{}
	userRepo := &mockUserRepo{}
	authSvc := auth.NewService(userRepo, teamRepo, 4)
	h := newUserHandler(authSvc, userRepo, teamRepo)

	body, _ := json.Marshal(map[string]interface{}{
		"name":   "alice",
		"teamId": "not-a-uuid",
	})

	req, w := makeChiRequest(http.MethodPost, "/users", body, "/users", nil)

	h.Create(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "VALIDATION_ERROR", errObj["code"])
}

func TestUserCreate_TeamNotFound(t *testing.T) {
	t.Parallel()

	teamID := uuid.New()
	teamRepo := &mockTeamRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*team.Team, error) {
			return nil, team.ErrTeamNotFound
		},
	}
	userRepo := &mockUserRepo{}
	authSvc := auth.NewService(userRepo, teamRepo, 4)
	h := newUserHandler(authSvc, userRepo, teamRepo)

	body, _ := json.Marshal(map[string]interface{}{
		"name":   "alice",
		"teamId": teamID.String(),
	})

	req, w := makeChiRequest(http.MethodPost, "/users", body, "/users", nil)

	h.Create(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

func TestUserCreate_InvalidJSON(t *testing.T) {
	t.Parallel()

	teamRepo := &mockTeamRepo{}
	userRepo := &mockUserRepo{}
	authSvc := auth.NewService(userRepo, teamRepo, 4)
	h := newUserHandler(authSvc, userRepo, teamRepo)

	req, w := makeChiRequest(http.MethodPost, "/users", []byte("{invalid"), "/users", nil)

	h.Create(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_JSON", errObj["code"])
}

// ===== GET /users =====

func TestUserList_Empty(t *testing.T) {
	t.Parallel()

	teamRepo := &mockTeamRepo{}
	userRepo := &mockUserRepo{}
	authSvc := auth.NewService(userRepo, teamRepo, 4)
	h := newUserHandler(authSvc, userRepo, teamRepo)

	req, w := makeChiRequest(http.MethodGet, "/users", nil, "/users", nil)

	h.List(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err)

	data := env["data"].([]interface{})
	assert.Len(t, data, 0)

	meta := env["meta"].(map[string]interface{})
	assert.Equal(t, float64(0), meta["total"])
}

func TestUserList_WithTeamInfo(t *testing.T) {
	t.Parallel()

	teamID := uuid.New()
	userID := uuid.New()

	teamRepo := &mockTeamRepo{
		getByIDFn: func(_ context.Context, id uuid.UUID) (*team.Team, error) {
			return &team.Team{
				ID:   teamID,
				Name: "ops",
				Role: "platform",
			}, nil
		},
	}
	userRepo := &mockUserRepo{
		listFn: func(_ context.Context) ([]auth.User, error) {
			return []auth.User{
				*sampleUser(userID, &teamID, false),
			}, nil
		},
	}
	authSvc := auth.NewService(userRepo, teamRepo, 4)
	h := newUserHandler(authSvc, userRepo, teamRepo)

	req, w := makeChiRequest(http.MethodGet, "/users", nil, "/users", nil)

	h.List(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err)

	data := env["data"].([]interface{})
	assert.Len(t, data, 1)

	user := data[0].(map[string]interface{})
	assert.Equal(t, userID.String(), user["id"])
	assert.Equal(t, "alice", user["name"])
	assert.Equal(t, teamID.String(), user["teamId"])
	assert.Equal(t, "ops", user["teamName"])
	assert.Equal(t, "platform", user["role"])
	assert.Equal(t, "daap_abc", user["apiKeyPrefix"])
	assert.Equal(t, false, user["isSuperuser"])
	assert.NotEmpty(t, user["createdAt"])

	// Should NOT have apiKey or apiKeyHash
	assert.Nil(t, user["apiKey"])
	assert.Nil(t, user["apiKeyHash"])
}

func TestUserList_SuperuserNoTeam(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	userRepo := &mockUserRepo{
		listFn: func(_ context.Context) ([]auth.User, error) {
			return []auth.User{
				*sampleUser(userID, nil, true),
			}, nil
		},
	}
	teamRepo := &mockTeamRepo{}
	authSvc := auth.NewService(userRepo, teamRepo, 4)
	h := newUserHandler(authSvc, userRepo, teamRepo)

	req, w := makeChiRequest(http.MethodGet, "/users", nil, "/users", nil)

	h.List(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err)

	data := env["data"].([]interface{})
	assert.Len(t, data, 1)

	user := data[0].(map[string]interface{})
	assert.Equal(t, true, user["isSuperuser"])
	assert.Nil(t, user["teamId"])
	assert.Nil(t, user["teamName"])
	assert.Nil(t, user["role"])
}

// ===== DELETE /users/{id} =====

func TestUserDelete_Success(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	teamID := uuid.New()
	userRepo := &mockUserRepo{
		getByIDFn: func(_ context.Context, id uuid.UUID) (*auth.User, error) {
			return sampleUser(userID, &teamID, false), nil
		},
		revokeFn: func(_ context.Context, id uuid.UUID) error {
			assert.Equal(t, userID, id)
			return nil
		},
	}
	teamRepo := &mockTeamRepo{}
	authSvc := auth.NewService(userRepo, teamRepo, 4)
	h := newUserHandler(authSvc, userRepo, teamRepo)

	req, w := makeChiRequest(http.MethodDelete, "/users/"+userID.String(), nil, "/users/{id}", map[string]string{"id": userID.String()})

	h.Delete(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestUserDelete_NotFound(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	userRepo := &mockUserRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*auth.User, error) {
			return nil, auth.ErrUserNotFound
		},
	}
	teamRepo := &mockTeamRepo{}
	authSvc := auth.NewService(userRepo, teamRepo, 4)
	h := newUserHandler(authSvc, userRepo, teamRepo)

	req, w := makeChiRequest(http.MethodDelete, "/users/"+userID.String(), nil, "/users/{id}", map[string]string{"id": userID.String()})

	h.Delete(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "NOT_FOUND", errObj["code"])
}

func TestUserDelete_CannotRevokeSuperuser(t *testing.T) {
	t.Parallel()

	userID := uuid.New()
	userRepo := &mockUserRepo{
		getByIDFn: func(_ context.Context, _ uuid.UUID) (*auth.User, error) {
			return sampleUser(userID, nil, true), nil
		},
	}
	teamRepo := &mockTeamRepo{}
	authSvc := auth.NewService(userRepo, teamRepo, 4)
	h := newUserHandler(authSvc, userRepo, teamRepo)

	req, w := makeChiRequest(http.MethodDelete, "/users/"+userID.String(), nil, "/users/{id}", map[string]string{"id": userID.String()})

	h.Delete(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "FORBIDDEN", errObj["code"])
	assert.Contains(t, errObj["message"], "superuser")
}

func TestUserDelete_InvalidID(t *testing.T) {
	t.Parallel()

	userRepo := &mockUserRepo{}
	teamRepo := &mockTeamRepo{}
	authSvc := auth.NewService(userRepo, teamRepo, 4)
	h := newUserHandler(authSvc, userRepo, teamRepo)

	req, w := makeChiRequest(http.MethodDelete, "/users/not-a-uuid", nil, "/users/{id}", map[string]string{"id": "not-a-uuid"})

	h.Delete(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	env := parseEnvelope(t, w)
	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "INVALID_ID", errObj["code"])
}
