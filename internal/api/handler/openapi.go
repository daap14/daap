package handler

import (
	"log/slog"
	"net/http"
	"sync"

	"sigs.k8s.io/yaml"
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
		http.Error(w, `{"error":"failed to convert spec"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(h.jsonSpec)
}
