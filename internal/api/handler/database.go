package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/daap14/daap/internal/api/middleware"
	"github.com/daap14/daap/internal/api/response"
	"github.com/daap14/daap/internal/api/validation"
	"github.com/daap14/daap/internal/database"
	"github.com/daap14/daap/internal/k8s"
	"github.com/daap14/daap/internal/k8s/template"
	"github.com/daap14/daap/internal/team"
)

// createDatabaseRequest is the request body for POST /databases.
type createDatabaseRequest struct {
	Name      string `json:"name"`
	OwnerTeam string `json:"ownerTeam"`
	Purpose   string `json:"purpose"`
	Namespace string `json:"namespace"`
}

// databaseResponse is the API representation of a database record.
type databaseResponse struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	OwnerTeam   string  `json:"ownerTeam"`
	Purpose     string  `json:"purpose"`
	Namespace   string  `json:"namespace"`
	ClusterName string  `json:"clusterName"`
	PoolerName  string  `json:"poolerName"`
	Status      string  `json:"status"`
	Host        *string `json:"host,omitempty"`
	Port        *int    `json:"port,omitempty"`
	SecretName  *string `json:"secretName,omitempty"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
}

// toDatabaseResponse converts a database model to its API response representation.
func toDatabaseResponse(db *database.Database) databaseResponse {
	resp := databaseResponse{
		ID:          db.ID.String(),
		Name:        db.Name,
		OwnerTeam:   db.OwnerTeamName,
		Purpose:     db.Purpose,
		Namespace:   db.Namespace,
		ClusterName: db.ClusterName,
		PoolerName:  db.PoolerName,
		Status:      db.Status,
		CreatedAt:   db.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   db.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
	if db.Status == "ready" {
		resp.Host = db.Host
		resp.Port = db.Port
		resp.SecretName = db.SecretName
	}
	return resp
}

// updateDatabaseRequest is the request body for PATCH /databases/:id.
type updateDatabaseRequest struct {
	Name      *string `json:"name,omitempty"`
	OwnerTeam *string `json:"ownerTeam,omitempty"`
	Purpose   *string `json:"purpose,omitempty"`
}

// DatabaseHandler handles database CRUD endpoints.
type DatabaseHandler struct {
	repo     database.Repository
	manager  k8s.ResourceManager
	teamRepo team.Repository
	ns       string
}

// NewDatabaseHandler creates a new DatabaseHandler.
func NewDatabaseHandler(repo database.Repository, manager k8s.ResourceManager, teamRepo team.Repository, ns string) *DatabaseHandler {
	return &DatabaseHandler{
		repo:     repo,
		manager:  manager,
		teamRepo: teamRepo,
		ns:       ns,
	}
}

// isProductUser returns true if the identity is a product-role user.
// Returns the user's team ID instead of team name for ownership comparisons.
func isProductUser(r *http.Request) (*uuid.UUID, bool) {
	identity := middleware.GetIdentity(r.Context())
	if identity != nil && identity.Role != nil && *identity.Role == "product" {
		return identity.TeamID, true
	}
	return nil, false
}

// Create handles POST /databases.
func (h *DatabaseHandler) Create(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	var req createDatabaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Err(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", requestID)
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.OwnerTeam = strings.TrimSpace(req.OwnerTeam)

	// Ownership scoping for product users
	identity := middleware.GetIdentity(r.Context())
	if identity != nil && identity.Role != nil && *identity.Role == "product" {
		if req.OwnerTeam == "" && identity.TeamName != nil {
			req.OwnerTeam = *identity.TeamName
		} else if identity.TeamName != nil && req.OwnerTeam != *identity.TeamName {
			response.Err(w, http.StatusForbidden, "FORBIDDEN", "Cannot create databases for another team", requestID)
			return
		}
	}

	fieldErrors := validation.ValidateCreateRequest(validation.CreateDatabaseRequest{
		Name:      req.Name,
		OwnerTeam: req.OwnerTeam,
	})
	if len(fieldErrors) > 0 {
		response.ErrWithDetails(w, http.StatusBadRequest, "VALIDATION_ERROR", "Input validation failed", fieldErrors, requestID)
		return
	}

	req.Purpose = strings.TrimSpace(req.Purpose)

	// Resolve ownerTeam name to team ID
	ownerTeam, err := h.teamRepo.GetByName(r.Context(), req.OwnerTeam)
	if err != nil {
		if errors.Is(err, team.ErrTeamNotFound) {
			response.Err(w, http.StatusNotFound, "NOT_FOUND", "Owner team not found", requestID)
			return
		}
		slog.Error("failed to look up owner team", "error", err)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create database", requestID)
		return
	}

	namespace := req.Namespace
	if namespace == "" {
		namespace = h.ns
	}

	db := &database.Database{
		Name:          req.Name,
		OwnerTeamID:   ownerTeam.ID,
		OwnerTeamName: ownerTeam.Name,
		Purpose:       req.Purpose,
		Namespace:     namespace,
	}

	if err := h.repo.Create(r.Context(), db); err != nil {
		if errors.Is(err, database.ErrDuplicateName) {
			response.Err(w, http.StatusConflict, "DUPLICATE_NAME", fmt.Sprintf("A database named %q already exists", req.Name), requestID)
			return
		}
		slog.Error("failed to create database record", "error", err)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create database", requestID)
		return
	}

	// TODO(v0.5): replace hardcoded defaults with tier params once handler accepts tier
	cluster := template.BuildCluster(template.ClusterParams{
		Name:        db.Name,
		Namespace:   db.Namespace,
		Instances:   1,
		CPU:         "500m",
		Memory:      "512Mi",
		StorageSize: "1Gi",
		PGVersion:   "16",
	})

	if err := h.manager.ApplyCluster(r.Context(), cluster); err != nil {
		slog.Error("failed to apply CNPG cluster", "error", err, "database", db.Name)
		h.markCreateError(r.Context(), db)
		response.Err(w, http.StatusInternalServerError, "K8S_ERROR", "Failed to create Kubernetes cluster resource", requestID)
		return
	}

	pooler := template.BuildPooler(template.PoolerParams{
		Name:        db.Name,
		Namespace:   db.Namespace,
		ClusterName: db.ClusterName,
	})

	if err := h.manager.ApplyPooler(r.Context(), pooler); err != nil {
		slog.Error("failed to apply CNPG pooler", "error", err, "database", db.Name)
		h.markCreateError(r.Context(), db)
		response.Err(w, http.StatusInternalServerError, "K8S_ERROR", "Failed to create Kubernetes pooler resource", requestID)
		return
	}

	response.Success(w, http.StatusCreated, toDatabaseResponse(db), requestID)
}

// List handles GET /databases.
func (h *DatabaseHandler) List(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	filter := database.ListFilter{
		Page:  1,
		Limit: 20,
	}

	// Ownership scoping: product users only see their own team's databases
	if teamID, ok := isProductUser(r); ok {
		filter.OwnerTeamID = teamID
	} else if v := r.URL.Query().Get("owner_team"); v != "" {
		// Resolve team name to ID for filter
		t, err := h.teamRepo.GetByName(r.Context(), v)
		if err != nil {
			if errors.Is(err, team.ErrTeamNotFound) {
				// No team with this name — return empty results
				response.SuccessList(w, http.StatusOK, []databaseResponse{}, 0, 1, 20, requestID)
				return
			}
			slog.Error("failed to look up team for filter", "error", err)
			response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list databases", requestID)
			return
		}
		filter.OwnerTeamID = &t.ID
	}

	if v := r.URL.Query().Get("status"); v != "" {
		filter.Status = &v
	}
	if v := r.URL.Query().Get("name"); v != "" {
		filter.Name = &v
	}
	if v := r.URL.Query().Get("page"); v != "" {
		page, err := strconv.Atoi(v)
		if err != nil || page < 1 {
			response.Err(w, http.StatusBadRequest, "INVALID_PARAM", "page must be a positive integer", requestID)
			return
		}
		filter.Page = page
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, err := strconv.Atoi(v)
		if err != nil || limit < 1 {
			response.Err(w, http.StatusBadRequest, "INVALID_PARAM", "limit must be a positive integer", requestID)
			return
		}
		filter.Limit = limit
	}

	result, err := h.repo.List(r.Context(), filter)
	if err != nil {
		slog.Error("failed to list databases", "error", err)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list databases", requestID)
		return
	}

	items := make([]databaseResponse, 0, len(result.Databases))
	for i := range result.Databases {
		items = append(items, toDatabaseResponse(&result.Databases[i]))
	}

	response.SuccessList(w, http.StatusOK, items, result.Total, result.Page, result.Limit, requestID)
}

// GetByID handles GET /databases/{id}.
func (h *DatabaseHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.Err(w, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID", requestID)
		return
	}

	db, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			response.Err(w, http.StatusNotFound, "NOT_FOUND", "Database not found", requestID)
			return
		}
		slog.Error("failed to get database", "error", err, "id", id)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get database", requestID)
		return
	}

	// Product users: return 404 for non-owned databases (no info leakage)
	if teamID, ok := isProductUser(r); ok {
		if db.OwnerTeamID != *teamID {
			response.Err(w, http.StatusNotFound, "NOT_FOUND", "Database not found", requestID)
			return
		}
	}

	response.Success(w, http.StatusOK, toDatabaseResponse(db), requestID)
}

// Update handles PATCH /databases/{id}.
func (h *DatabaseHandler) Update(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.Err(w, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID", requestID)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	var req updateDatabaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Err(w, http.StatusBadRequest, "INVALID_JSON", "Request body must be valid JSON", requestID)
		return
	}

	if req.Name != nil {
		response.Err(w, http.StatusBadRequest, "IMMUTABLE_FIELD", "name is immutable", requestID)
		return
	}

	// Product users: check ownership and cannot change ownerTeam
	if teamID, ok := isProductUser(r); ok {
		if req.OwnerTeam != nil {
			response.Err(w, http.StatusForbidden, "FORBIDDEN", "Product users cannot change ownerTeam", requestID)
			return
		}
		// Verify ownership
		existing, err := h.repo.GetByID(r.Context(), id)
		if err != nil {
			if errors.Is(err, database.ErrNotFound) {
				response.Err(w, http.StatusNotFound, "NOT_FOUND", "Database not found", requestID)
				return
			}
			slog.Error("failed to get database for ownership check", "error", err, "id", id)
			response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update database", requestID)
			return
		}
		if existing.OwnerTeamID != *teamID {
			response.Err(w, http.StatusNotFound, "NOT_FOUND", "Database not found", requestID)
			return
		}
	}

	// Resolve ownerTeam name to UUID if provided
	var updateFields database.UpdateFields
	if req.OwnerTeam != nil {
		t, err := h.teamRepo.GetByName(r.Context(), *req.OwnerTeam)
		if err != nil {
			if errors.Is(err, team.ErrTeamNotFound) {
				response.Err(w, http.StatusNotFound, "NOT_FOUND", "Owner team not found", requestID)
				return
			}
			slog.Error("failed to look up owner team", "error", err)
			response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update database", requestID)
			return
		}
		updateFields.OwnerTeamID = &t.ID
	}
	updateFields.Purpose = req.Purpose

	db, err := h.repo.Update(r.Context(), id, updateFields)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			response.Err(w, http.StatusNotFound, "NOT_FOUND", "Database not found", requestID)
			return
		}
		slog.Error("failed to update database", "error", err, "id", id)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to update database", requestID)
		return
	}

	response.Success(w, http.StatusOK, toDatabaseResponse(db), requestID)
}

// Delete handles DELETE /databases/{id}.
func (h *DatabaseHandler) Delete(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.GetRequestID(r.Context())

	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		response.Err(w, http.StatusBadRequest, "INVALID_ID", "id must be a valid UUID", requestID)
		return
	}

	db, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, database.ErrNotFound) {
			response.Err(w, http.StatusNotFound, "NOT_FOUND", "Database not found", requestID)
			return
		}
		slog.Error("failed to get database for deletion", "error", err, "id", id)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete database", requestID)
		return
	}

	// Product users: can only delete own databases
	if teamID, ok := isProductUser(r); ok {
		if db.OwnerTeamID != *teamID {
			response.Err(w, http.StatusNotFound, "NOT_FOUND", "Database not found", requestID)
			return
		}
	}

	if err := h.manager.DeleteCluster(r.Context(), db.Namespace, db.ClusterName); err != nil {
		slog.Error("failed to delete CNPG cluster", "error", err, "database", db.Name)
	}

	if err := h.manager.DeletePooler(r.Context(), db.Namespace, db.PoolerName); err != nil {
		slog.Error("failed to delete CNPG pooler", "error", err, "database", db.Name)
	}

	if err := h.repo.SoftDelete(r.Context(), id); err != nil {
		if errors.Is(err, database.ErrNotFound) {
			response.Err(w, http.StatusNotFound, "NOT_FOUND", "Database not found", requestID)
			return
		}
		slog.Error("failed to soft-delete database", "error", err, "id", id)
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete database", requestID)
		return
	}

	response.NoContent(w)
}

// markCreateError sets a newly created database record to "error" status
// when Kubernetes resource creation fails.
func (h *DatabaseHandler) markCreateError(ctx context.Context, db *database.Database) {
	if _, err := h.repo.UpdateStatus(ctx, db.ID, database.StatusUpdate{
		Status: "error",
	}); err != nil {
		slog.Error("failed to mark database as error after K8s failure", "error", err, "database", db.Name)
	}
}
