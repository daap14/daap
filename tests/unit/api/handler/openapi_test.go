package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	specpkg "github.com/daap14/daap/api"
	"github.com/daap14/daap/internal/api/handler"
)

func TestOpenAPIHandler_ReturnsJSON(t *testing.T) {
	t.Parallel()

	yamlSpec := []byte(`openapi: "3.1.0"
info:
  title: Test API
  version: "1.0.0"
paths: {}
`)
	h := handler.NewOpenAPIHandler(yamlSpec)
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err, "response should be valid JSON")

	assert.Equal(t, "3.1.0", result["openapi"])
	info := result["info"].(map[string]interface{})
	assert.Equal(t, "Test API", info["title"])
	assert.Equal(t, "1.0.0", info["version"])
	assert.Contains(t, result, "paths")
}

func TestOpenAPIHandler_ContainsTopLevelFields(t *testing.T) {
	t.Parallel()

	yamlSpec := []byte(`openapi: "3.1.0"
info:
  title: DAAP API
  version: "0.3.0"
paths:
  /health:
    get:
      summary: Health check
components:
  schemas: {}
`)
	h := handler.NewOpenAPIHandler(yamlSpec)
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err)

	assert.Contains(t, result, "openapi")
	assert.Contains(t, result, "info")
	assert.Contains(t, result, "paths")
	assert.Contains(t, result, "components")
}

func TestOpenAPIHandler_InvalidYAML_Returns500(t *testing.T) {
	t.Parallel()

	invalidYAML := []byte(`{{{not yaml at all}}}`)
	h := handler.NewOpenAPIHandler(invalidYAML)
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err, "error response should be valid JSON envelope")

	assert.Nil(t, env["data"])
	assert.NotNil(t, env["error"])

	errObj := env["error"].(map[string]interface{})
	assert.Equal(t, "INTERNAL_ERROR", errObj["code"])
}

func TestOpenAPIHandler_CachesConversion(t *testing.T) {
	t.Parallel()

	yamlSpec := []byte(`openapi: "3.1.0"
info:
  title: Cache Test
  version: "1.0.0"
paths: {}
`)
	h := handler.NewOpenAPIHandler(yamlSpec)

	// First request
	req1 := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	w1 := httptest.NewRecorder()
	h.ServeHTTP(w1, req1)

	// Second request
	req2 := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)

	assert.Equal(t, http.StatusOK, w1.Code)
	assert.Equal(t, http.StatusOK, w2.Code)
	assert.Equal(t, w1.Body.String(), w2.Body.String(), "cached response should be identical")
}

func TestOpenAPIHandler_EmbeddedSpec(t *testing.T) {
	t.Parallel()

	require.NotEmpty(t, specpkg.OpenAPISpec, "embedded OpenAPI spec should not be empty")

	h := handler.NewOpenAPIHandler(specpkg.OpenAPISpec)
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	require.NoError(t, err, "embedded spec should produce valid JSON")

	assert.Equal(t, "3.1.0", result["openapi"])
	info := result["info"].(map[string]interface{})
	assert.Equal(t, "DAAP API", info["title"])
	assert.Equal(t, "0.6.0", info["version"])
}
