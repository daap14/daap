package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daap14/daap/internal/api/middleware"
)

func TestRecovery_NoPanic(t *testing.T) {
	// Arrange
	handler := middleware.Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRecovery_HandlesPanic(t *testing.T) {
	// Arrange
	handler := middleware.Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("something went wrong")
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err)

	assert.Nil(t, env["data"])
	assert.NotNil(t, env["error"])

	apiErr := env["error"].(map[string]interface{})
	assert.Equal(t, "INTERNAL_ERROR", apiErr["code"])
	assert.Equal(t, "An unexpected error occurred", apiErr["message"])
}

func TestRecovery_HandlesPanicWithRequestID(t *testing.T) {
	// Arrange: chain RequestID -> Recovery -> panicking handler
	panicker := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})
	handler := middleware.RequestID(middleware.Recovery(panicker))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err)

	meta := env["meta"].(map[string]interface{})
	assert.NotEmpty(t, meta["requestId"])
	assert.Equal(t, w.Header().Get("X-Request-ID"), meta["requestId"])
}

func TestRecovery_HandlesPanicWithNilValue(t *testing.T) {
	handler := middleware.Recovery(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(nil)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	// panic(nil) in Go 1.21+ triggers a runtime.PanicNilError, which recover() catches.
	// The recovery middleware should handle this gracefully.
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
