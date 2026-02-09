package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daap14/daap/internal/api/middleware"
)

func TestRequestID_GeneratesNewID(t *testing.T) {
	// Arrange
	var capturedID string
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = middleware.GetRequestID(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	assert.NotEmpty(t, capturedID)
	_, err := uuid.Parse(capturedID)
	assert.NoError(t, err, "generated request ID should be a valid UUID")
	assert.Equal(t, capturedID, w.Header().Get("X-Request-ID"))
}

func TestRequestID_UsesExistingHeader(t *testing.T) {
	// Arrange
	existingID := "my-existing-request-id"
	var capturedID string
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = middleware.GetRequestID(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", existingID)
	w := httptest.NewRecorder()

	// Act
	handler.ServeHTTP(w, req)

	// Assert
	assert.Equal(t, existingID, capturedID)
	assert.Equal(t, existingID, w.Header().Get("X-Request-ID"))
}

func TestRequestID_SetsResponseHeader(t *testing.T) {
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	responseID := w.Header().Get("X-Request-ID")
	require.NotEmpty(t, responseID)
	_, err := uuid.Parse(responseID)
	assert.NoError(t, err)
}

func TestRequestID_UniquePerRequest(t *testing.T) {
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ids := make(map[string]bool)

	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		id := w.Header().Get("X-Request-ID")
		assert.False(t, ids[id], "request IDs should be unique")
		ids[id] = true
	}
}

func TestGetRequestID_EmptyContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	id := middleware.GetRequestID(req.Context())

	assert.Equal(t, "", id)
}
