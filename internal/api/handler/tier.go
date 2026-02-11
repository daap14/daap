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
	"github.com/daap14/daap/internal/tier"
)

// createTierRequest is the request body for POST /tiers.
type createTierRequest struct {
	Name                string `json:"name"`
	Description         string `json:"description"`
	BlueprintName       string `json:"blueprintName"`
	DestructionStrategy string `json:"destructionStrategy"`
	BackupEnabled       bool   `json:"backupEnabled"`
}

// updateTierRequest is the request body for PATCH /tiers/{id}.
type updateTierRequest struct {
	Name                *string    `json:"name"`
	Description         *string    `json:"description"`
	BlueprintID         *uuid.UUID `json:"blueprintId"`
	DestructionStrategy *string    `json:"destructionStrategy"`
	BackupEnabled       *bool      `json:"backupEnabled"`
}

// tierResponse is the full API representation (platform users).
type tierResponse struct {
	ID                  string  `json:"id"`
	Name                string  `json:"name"`
	Description         string  `json:"description"`
	BlueprintID         *string `json:"blueprintId,omitempty"`
	BlueprintName       string  `json:"blueprintName,omitempty"`
	DestructionStrategy string  `json:"destructionStrategy"`
	BackupEnabled       bool    `json:"backupEnabled"`
	CreatedAt           string  `json:"createdAt"`
	UpdatedAt           string  `json:"updatedAt"`
}

// tierSummaryResponse is the redacted API representation (product users).
type tierSummaryResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func toTierResponse(t *tier.Tier) tierResponse {
	resp := tierResponse{
		ID:                  t.ID.String(),
		Name:                t.Name,
		Description:         t.Description,
		BlueprintName:       t.BlueprintName,
		DestructionStrategy: t.DestructionStrategy,
		BackupEnabled:       t.BackupEnabled,
		CreatedAt:           t.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:           t.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
	if t.BlueprintID != nil {
		s := t.BlueprintID.String()
		resp.BlueprintID = &s
	}
	return resp
}

func toTierSummaryResponse(t *tier.Tier) tierSummaryResponse {
	return tierSummaryResponse{
		ID:          t.ID.String(),
		Name:        t.Name,
		Description: t.Description,
	}
}

// TierHandler handles tier CRUD endpoints.
type TierHandler struct {
	repo   tier.Repository
	bpRepo blueprint.Repository
}

// NewTierHandler creates a new TierHandler.
func NewTierHandler(repo tier.Repository, bpRepo blueprint.Repository) *TierHandler {
	return &TierHandler{repo: repo, bpRepo: bpRepo}
}

// Create handles POST /tiers.
func (h *TierHandler) Create(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req createTierRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Err(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", requestID)
		return
	}

	req.Name = strings.TrimSpace(req.Name)

	fieldErrors := validation.ValidateCreateTierRequest(validation.CreateTierRequest{
		Name:                req.Name,
		Description:         req.Description,
		BlueprintName:       req.BlueprintName,
		DestructionStrategy: req.DestructionStrategy,
		BackupEnabled:       req.BackupEnabled,
	})
	if len(fieldErrors) > 0 {
		response.ErrWithDetails(w, http.StatusBadRequest, "VALIDATION_ERROR", "Input validation failed", fieldErrors, requestID)
		return
	}

	// Resolve blueprint by name if provided
	var blueprintID *uuid.UUID
	if req.BlueprintName != "" {
		bp, err := h.bpRepo.GetByName(r.Context(), req.BlueprintName)
		if err != nil {
			if errors.Is(err, blueprint.ErrBlueprintNotFound) {
				response.Err(w, http.StatusNotFound, "NOT_FOUND", "Blueprint not found", requestID)
				return
			}
			slog.Error("failed to look up blueprint", "error", err)
			response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create tier", requestID)
			return
		}
		blueprintID = &bp.ID
	}

	t := &tier.Tier{
		Name:                req.Name,
		Description:         req.Description,
		BlueprintID:         blueprintID,
		DestructionStrategy: req.DestructionStrategy,
		BackupEnabled:       req.BackupEnabled,
	}

	if err := h.repo.Create(r.Context(), t); err != nil {
		if errors.Is(err, tier.ErrDuplicateTierName) {
			response.Err(w, http.StatusConflict, "DUPLICATE_NAME", fmt.Sprintf("A tier named %q already exists", req.Name), requestID)
			return
		}
		slog.Error("failed to create tier", "error", err)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create tier", requestID)
		return
	}

	response.Success(w, http.StatusCreated, toTierResponse(t), requestID)
}

// List handles GET /tiers.
func (h *TierHandler) List(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	tiers, err := h.repo.List(r.Context())
	if err != nil {
		slog.Error("failed to list tiers", "error", err)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list tiers", requestID)
		return
	}

	identity := middleware.GetIdentity(r.Context())
	if identity != nil && identity.Role != nil && *identity.Role == "product" {
		items := make([]tierSummaryResponse, 0, len(tiers))
		for i := range tiers {
			items = append(items, toTierSummaryResponse(&tiers[i]))
		}
		response.SuccessList(w, http.StatusOK, items, len(items), 1, 100, requestID)
		return
	}

	items := make([]tierResponse, 0, len(tiers))
	for i := range tiers {
		items = append(items, toTierResponse(&tiers[i]))
	}
	response.SuccessList(w, http.StatusOK, items, len(items), 1, 100, requestID)
}

// GetByID handles GET /tiers/{id}.
func (h *TierHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.Err(w, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID", requestID)
		return
	}

	t, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, tier.ErrTierNotFound) {
			response.Err(w, http.StatusNotFound, "NOT_FOUND", "Tier not found", requestID)
			return
		}
		slog.Error("failed to get tier", "error", err, "id", id)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get tier", requestID)
		return
	}

	identity := middleware.GetIdentity(r.Context())
	if identity != nil && identity.Role != nil && *identity.Role == "product" {
		response.Success(w, http.StatusOK, toTierSummaryResponse(t), requestID)
		return
	}

	response.Success(w, http.StatusOK, toTierResponse(t), requestID)
}

// Update handles PATCH /tiers/{id}.
func (h *TierHandler) Update(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.Err(w, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID", requestID)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req updateTierRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Err(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", requestID)
		return
	}

	if req.Name != nil {
		response.Err(w, http.StatusBadRequest, "IMMUTABLE_FIELD", "name cannot be changed", requestID)
		return
	}

	fieldErrors := validation.ValidateUpdateTierRequest(validation.UpdateTierRequest{
		Description:         req.Description,
		DestructionStrategy: req.DestructionStrategy,
		BackupEnabled:       req.BackupEnabled,
	})
	if len(fieldErrors) > 0 {
		response.ErrWithDetails(w, http.StatusBadRequest, "VALIDATION_ERROR", "Input validation failed", fieldErrors, requestID)
		return
	}

	fields := tier.UpdateFields{
		Description:         req.Description,
		BlueprintID:         req.BlueprintID,
		DestructionStrategy: req.DestructionStrategy,
		BackupEnabled:       req.BackupEnabled,
	}

	t, err := h.repo.Update(r.Context(), id, fields)
	if err != nil {
		if errors.Is(err, tier.ErrTierNotFound) {
			response.Err(w, http.StatusNotFound, "NOT_FOUND", "Tier not found", requestID)
			return
		}
		slog.Error("failed to update tier", "error", err, "id", id)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update tier", requestID)
		return
	}

	response.Success(w, http.StatusOK, toTierResponse(t), requestID)
}

// Delete handles DELETE /tiers/{id}.
func (h *TierHandler) Delete(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.Err(w, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID", requestID)
		return
	}

	if err := h.repo.Delete(r.Context(), id); err != nil {
		if errors.Is(err, tier.ErrTierNotFound) {
			response.Err(w, http.StatusNotFound, "NOT_FOUND", "Tier not found", requestID)
			return
		}
		if errors.Is(err, tier.ErrTierHasDatabases) {
			response.Err(w, http.StatusConflict, "TIER_HAS_DATABASES", "Cannot delete tier with active databases", requestID)
			return
		}
		slog.Error("failed to delete tier", "error", err, "id", id)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete tier", requestID)
		return
	}

	response.NoContent(w)
}
