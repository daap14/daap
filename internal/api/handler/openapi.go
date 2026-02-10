package handler

import (
	"log/slog"
	"net/http"
	"sync"

	"sigs.k8s.io/yaml"

	"github.com/daap14/daap/internal/api/middleware"
	"github.com/daap14/daap/internal/api/response"
)

// OpenAPIHandler serves the OpenAPI spec as JSON.
type OpenAPIHandler struct {
	rawYAML  []byte
	jsonOnce sync.Once
	jsonSpec []byte
	jsonErr  error
}

// NewOpenAPIHandler creates a handler that converts the YAML spec to JSON on first request.
func NewOpenAPIHandler(yamlSpec []byte) *OpenAPIHandler {
	return &OpenAPIHandler{rawYAML: yamlSpec}
}

// ServeHTTP converts the embedded YAML spec to JSON (cached) and writes it.
func (h *OpenAPIHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.jsonOnce.Do(func() {
		h.jsonSpec, h.jsonErr = yaml.YAMLToJSON(h.rawYAML)
	})

	if h.jsonErr != nil {
		slog.Error("failed to convert OpenAPI spec to JSON", "error", h.jsonErr)
		requestID := middleware.GetRequestID(r.Context())
		response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to convert OpenAPI spec", requestID)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(h.jsonSpec); err != nil {
		slog.Error("failed to write OpenAPI spec response", "error", err)
	}
}
