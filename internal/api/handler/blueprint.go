package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/daap14/daap/internal/api/middleware"
	"github.com/daap14/daap/internal/api/response"
	"github.com/daap14/daap/internal/api/validation"
	"github.com/daap14/daap/internal/blueprint"
	"github.com/daap14/daap/internal/provider"
)

// createBlueprintRequest is the request body for POST /blueprints.
type createBlueprintRequest struct {
	Name      string `json:"name"`
	Provider  string `json:"provider"`
	Manifests string `json:"manifests"`
}

// blueprintResponse is the API representation of a blueprint.
type blueprintResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Provider  string `json:"provider"`
	Manifests string `json:"manifests"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

func toBlueprintResponse(bp *blueprint.Blueprint) blueprintResponse {
	return blueprintResponse{
		ID:        bp.ID.String(),
		Name:      bp.Name,
		Provider:  bp.Provider,
		Manifests: bp.Manifests,
		CreatedAt: bp.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt: bp.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// BlueprintHandler handles blueprint CRUD endpoints.
type BlueprintHandler struct {
	repo     blueprint.Repository
	registry *provider.Registry
}

// NewBlueprintHandler creates a new BlueprintHandler.
func NewBlueprintHandler(repo blueprint.Repository, registry *provider.Registry) *BlueprintHandler {
	return &BlueprintHandler{repo: repo, registry: registry}
}

// Create handles POST /blueprints.
func (h *BlueprintHandler) Create(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req createBlueprintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Err(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", requestID)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Provider = strings.TrimSpace(req.Provider)

	fieldErrors := validation.ValidateCreateBlueprintRequest(validation.CreateBlueprintRequest{
		Name:      req.Name,
		Provider:  req.Provider,
		Manifests: req.Manifests,
		Registry:  h.registry,
	})
	if len(fieldErrors) > 0 {
		response.ErrWithDetails(w, http.StatusBadRequest, "VALIDATION_ERROR", "Input validation failed", fieldErrors, requestID)
		return
	}

	bp := &blueprint.Blueprint{
		Name:      req.Name,
		Provider:  req.Provider,
		Manifests: req.Manifests,
	}

	if err := h.repo.Create(r.Context(), bp); err != nil {
		if errors.Is(err, blueprint.ErrDuplicateBlueprintName) {
			response.Err(w, http.StatusConflict, "DUPLICATE_NAME", fmt.Sprintf("A blueprint named %q already exists", req.Name), requestID)
			return
		}
		slog.Error("failed to create blueprint", "error", err)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create blueprint", requestID)
		return
	}

	response.Success(w, http.StatusCreated, toBlueprintResponse(bp), requestID)
}

// List handles GET /blueprints.
func (h *BlueprintHandler) List(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	blueprints, err := h.repo.List(r.Context())
	if err != nil {
		slog.Error("failed to list blueprints", "error", err)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list blueprints", requestID)
		return
	}

	items := make([]blueprintResponse, 0, len(blueprints))
	for i := range blueprints {
		items = append(items, toBlueprintResponse(&blueprints[i]))
	}

	response.SuccessList(w, http.StatusOK, items, len(items), 1, 100, requestID)
}

// GetByID handles GET /blueprints/{id}.
func (h *BlueprintHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.Err(w, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID", requestID)
		return
	}

	bp, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, blueprint.ErrBlueprintNotFound) {
			response.Err(w, http.StatusNotFound, "NOT_FOUND", "Blueprint not found", requestID)
			return
		}
		slog.Error("failed to get blueprint", "error", err, "id", id)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get blueprint", requestID)
		return
	}

	response.Success(w, http.StatusOK, toBlueprintResponse(bp), requestID)
}

// Delete handles DELETE /blueprints/{id}.
func (h *BlueprintHandler) Delete(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.Err(w, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID", requestID)
		return
	}

	if err := h.repo.Delete(r.Context(), id); err != nil {
		if errors.Is(err, blueprint.ErrBlueprintNotFound) {
			response.Err(w, http.StatusNotFound, "NOT_FOUND", "Blueprint not found", requestID)
			return
		}
		if errors.Is(err, blueprint.ErrBlueprintHasTiers) {
			response.Err(w, http.StatusConflict, "BLUEPRINT_HAS_TIERS", "Cannot delete blueprint with active tiers", requestID)
			return
		}
		slog.Error("failed to delete blueprint", "error", err, "id", id)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete blueprint", requestID)
		return
	}

	response.NoContent(w)
}
