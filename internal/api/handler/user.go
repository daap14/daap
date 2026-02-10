package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/daap14/daap/internal/api/middleware"
	"github.com/daap14/daap/internal/api/response"
	"github.com/daap14/daap/internal/api/validation"
	"github.com/daap14/daap/internal/auth"
	"github.com/daap14/daap/internal/team"
)

type createUserRequest struct {
	Name   string `json:"name"`
	TeamID string `json:"teamId"`
}

type userResponse struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	TeamID       *string `json:"teamId,omitempty"`
	TeamName     *string `json:"teamName,omitempty"`
	Role         *string `json:"role,omitempty"`
	ApiKeyPrefix string  `json:"apiKeyPrefix"`
	IsSuperuser  bool    `json:"isSuperuser"`
	CreatedAt    string  `json:"createdAt"`
	RevokedAt    *string `json:"revokedAt,omitempty"`
}

type userWithKeyResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	TeamID    string `json:"teamId"`
	TeamName  string `json:"teamName"`
	Role      string `json:"role"`
	ApiKey    string `json:"apiKey"`
	CreatedAt string `json:"createdAt"`
}

// UserHandler handles user CRUD endpoints.
type UserHandler struct {
	authService *auth.Service
	userRepo    auth.UserRepository
	teamRepo    team.Repository
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(authService *auth.Service, userRepo auth.UserRepository, teamRepo team.Repository) *UserHandler {
	return &UserHandler{
		authService: authService,
		userRepo:    userRepo,
		teamRepo:    teamRepo,
	}
}

// Create handles POST /users.
func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Err(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", requestID)
		return
	}

	fieldErrors := validation.ValidateCreateUserRequest(validation.CreateUserRequest{
		Name:   req.Name,
		TeamID: req.TeamID,
	})
	if len(fieldErrors) > 0 {
		response.ErrWithDetails(w, http.StatusBadRequest, "VALIDATION_ERROR", "Input validation failed", fieldErrors, requestID)
		return
	}

	req.Name = strings.TrimSpace(req.Name)

	teamID, _ := uuid.Parse(req.TeamID) // already validated

	t, err := h.teamRepo.GetByID(r.Context(), teamID)
	if err != nil {
		if errors.Is(err, team.ErrTeamNotFound) {
			response.Err(w, http.StatusNotFound, "NOT_FOUND", "Team not found", requestID)
			return
		}
		slog.Error("failed to get team", "error", err)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create user", requestID)
		return
	}

	rawKey, prefix, hash, err := h.authService.GenerateKey()
	if err != nil {
		slog.Error("failed to generate API key", "error", err)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create user", requestID)
		return
	}

	u := &auth.User{
		Name:         req.Name,
		TeamID:       &teamID,
		IsSuperuser:  false,
		ApiKeyPrefix: prefix,
		ApiKeyHash:   hash,
	}

	if err := h.userRepo.Create(r.Context(), u); err != nil {
		slog.Error("failed to create user", "error", err)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create user", requestID)
		return
	}

	response.Success(w, http.StatusCreated, userWithKeyResponse{
		ID:        u.ID.String(),
		Name:      u.Name,
		TeamID:    teamID.String(),
		TeamName:  t.Name,
		Role:      t.Role,
		ApiKey:    rawKey,
		CreatedAt: u.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}, requestID)
}

// List handles GET /users.
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	users, err := h.userRepo.List(r.Context())
	if err != nil {
		slog.Error("failed to list users", "error", err)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list users", requestID)
		return
	}

	items := make([]userResponse, 0, len(users))
	for i := range users {
		u := &users[i]
		resp := userResponse{
			ID:           u.ID.String(),
			Name:         u.Name,
			ApiKeyPrefix: u.ApiKeyPrefix,
			IsSuperuser:  u.IsSuperuser,
			CreatedAt:    u.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		}
		if u.TeamID != nil {
			tid := u.TeamID.String()
			resp.TeamID = &tid
			resp.TeamName = u.TeamName
			resp.Role = u.TeamRole
		}
		if u.RevokedAt != nil {
			revoked := u.RevokedAt.UTC().Format("2006-01-02T15:04:05Z")
			resp.RevokedAt = &revoked
		}
		items = append(items, resp)
	}

	response.SuccessList(w, http.StatusOK, items, len(items), 1, 100, requestID)
}

// Delete handles DELETE /users/{id} (soft-revoke).
func (h *UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.Err(w, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID", requestID)
		return
	}

	// Check if user is the superuser — cannot revoke
	u, err := h.userRepo.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			response.Err(w, http.StatusNotFound, "NOT_FOUND", "User not found", requestID)
			return
		}
		slog.Error("failed to get user", "error", err, "id", id)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to revoke user", requestID)
		return
	}

	if u.IsSuperuser {
		response.Err(w, http.StatusForbidden, "FORBIDDEN", "Cannot revoke the superuser", requestID)
		return
	}

	if err := h.userRepo.Revoke(r.Context(), id); err != nil {
		if errors.Is(err, auth.ErrUserNotFound) {
			response.Err(w, http.StatusNotFound, "NOT_FOUND", "User not found", requestID)
			return
		}
		if errors.Is(err, auth.ErrUserRevoked) {
			// Already revoked — treat as success (idempotent)
			response.NoContent(w)
			return
		}
		slog.Error("failed to revoke user", "error", err, "id", id)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to revoke user", requestID)
		return
	}

	response.NoContent(w)
}
