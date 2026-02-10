package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/daap14/daap/internal/api/middleware"
	"github.com/daap14/daap/internal/api/response"
	"github.com/daap14/daap/internal/api/validation"
	"github.com/daap14/daap/internal/team"
)

type createTeamRequest struct {
	Name string `json:"name"`
	Role string `json:"role"`
}

type teamResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Role      string `json:"role"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func toTeamResponse(t *team.Team) teamResponse {
	return teamResponse{
		ID:        t.ID.String(),
		Name:      t.Name,
		Role:      t.Role,
		CreatedAt: t.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt: t.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// TeamHandler handles team CRUD endpoints.
type TeamHandler struct {
	repo team.Repository
}

// NewTeamHandler creates a new TeamHandler.
func NewTeamHandler(repo team.Repository) *TeamHandler {
	return &TeamHandler{repo: repo}
}

// Create handles POST /teams.
func (h *TeamHandler) Create(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req createTeamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Err(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", requestID)
		return
	}

	fieldErrors := validation.ValidateCreateTeamRequest(validation.CreateTeamRequest{
		Name: req.Name,
		Role: req.Role,
	})
	if len(fieldErrors) > 0 {
		response.ErrWithDetails(w, http.StatusBadRequest, "VALIDATION_ERROR", "Input validation failed", fieldErrors, requestID)
		return
	}

	t := &team.Team{
		Name: req.Name,
		Role: req.Role,
	}

	if err := h.repo.Create(r.Context(), t); err != nil {
		if errors.Is(err, team.ErrDuplicateTeamName) {
			response.Err(w, http.StatusConflict, "DUPLICATE_NAME", fmt.Sprintf("A team named %q already exists", req.Name), requestID)
			return
		}
		slog.Error("failed to create team", "error", err)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create team", requestID)
		return
	}

	response.Success(w, http.StatusCreated, toTeamResponse(t), requestID)
}

// List handles GET /teams.
func (h *TeamHandler) List(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	teams, err := h.repo.List(r.Context())
	if err != nil {
		slog.Error("failed to list teams", "error", err)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list teams", requestID)
		return
	}

	items := make([]teamResponse, 0, len(teams))
	for i := range teams {
		items = append(items, toTeamResponse(&teams[i]))
	}

	response.SuccessList(w, http.StatusOK, items, len(items), 1, 100, requestID)
}

// Delete handles DELETE /teams/{id}.
func (h *TeamHandler) Delete(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.Err(w, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID", requestID)
		return
	}

	if err := h.repo.Delete(r.Context(), id); err != nil {
		if errors.Is(err, team.ErrTeamNotFound) {
			response.Err(w, http.StatusNotFound, "NOT_FOUND", "Team not found", requestID)
			return
		}
		if errors.Is(err, team.ErrTeamHasUsers) {
			response.Err(w, http.StatusConflict, "TEAM_HAS_USERS", "Cannot delete team with active users", requestID)
			return
		}
		slog.Error("failed to delete team", "error", err, "id", id)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete team", requestID)
		return
	}

	response.NoContent(w)
}
