package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daap14/daap/internal/api/handler"
	"github.com/daap14/daap/internal/k8s"
)

// mockHealthChecker implements k8s.HealthChecker for testing.
type mockHealthChecker struct {
	status k8s.ConnectivityStatus
}

func (m *mockHealthChecker) CheckConnectivity(_ context.Context) k8s.ConnectivityStatus {
	return m.status
}

func TestHealthHandler_Healthy(t *testing.T) {
	// Arrange
	checker := &mockHealthChecker{
		status: k8s.ConnectivityStatus{
			Connected: true,
			Version:   "v1.31.0",
		},
	}
	h := handler.NewHealthHandler(checker, "0.1.0")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	// Act
	h.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err)

	data := env["data"].(map[string]interface{})
	assert.Equal(t, "healthy", data["status"])
	assert.Equal(t, "0.1.0", data["version"])

	k8sStatus := data["kubernetes"].(map[string]interface{})
	assert.Equal(t, true, k8sStatus["connected"])
	assert.Equal(t, "v1.31.0", k8sStatus["version"])

	assert.Nil(t, env["error"])
	assert.NotNil(t, env["meta"])
}

func TestHealthHandler_Degraded(t *testing.T) {
	// Arrange
	checker := &mockHealthChecker{
		status: k8s.ConnectivityStatus{
			Connected: false,
		},
	}
	h := handler.NewHealthHandler(checker, "0.1.0")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	// Act
	h.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)

	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err)

	data := env["data"].(map[string]interface{})
	assert.Equal(t, "degraded", data["status"])
	assert.Equal(t, "0.1.0", data["version"])

	k8sStatus := data["kubernetes"].(map[string]interface{})
	assert.Equal(t, false, k8sStatus["connected"])
	assert.Nil(t, k8sStatus["version"])
}

func TestHealthHandler_VersionReflectsConfig(t *testing.T) {
	checker := &mockHealthChecker{
		status: k8s.ConnectivityStatus{Connected: true, Version: "v1.30.0"},
	}
	h := handler.NewHealthHandler(checker, "2.5.0-beta")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err)

	data := env["data"].(map[string]interface{})
	assert.Equal(t, "2.5.0-beta", data["version"])
}

func TestHealthHandler_ResponseEnvelopeStructure(t *testing.T) {
	// Verify the full envelope matches the OpenAPI spec
	checker := &mockHealthChecker{
		status: k8s.ConnectivityStatus{Connected: true, Version: "v1.31.0"},
	}
	h := handler.NewHealthHandler(checker, "0.1.0")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err)

	// Top-level keys: data, error, meta
	assert.Contains(t, env, "data")
	assert.Contains(t, env, "error")
	assert.Contains(t, env, "meta")

	// Meta has requestId and timestamp
	meta := env["meta"].(map[string]interface{})
	assert.Contains(t, meta, "requestId")
	assert.Contains(t, meta, "timestamp")

	// Data has status, version, kubernetes
	data := env["data"].(map[string]interface{})
	assert.Contains(t, data, "status")
	assert.Contains(t, data, "version")
	assert.Contains(t, data, "kubernetes")

	// Kubernetes has connected and version
	k8sData := data["kubernetes"].(map[string]interface{})
	assert.Contains(t, k8sData, "connected")
	assert.Contains(t, k8sData, "version")
}

func TestHealthHandler_DevVersion(t *testing.T) {
	// Default version when unset is "dev"
	checker := &mockHealthChecker{
		status: k8s.ConnectivityStatus{Connected: false},
	}
	h := handler.NewHealthHandler(checker, "dev")
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err)

	data := env["data"].(map[string]interface{})
	assert.Equal(t, "dev", data["version"])
}
