package response_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/daap14/daap/internal/api/response"
)

func TestNewMeta_GeneratesUUID(t *testing.T) {
	meta := response.NewMeta("")

	_, err := uuid.Parse(meta.RequestID)
	assert.NoError(t, err, "requestId should be a valid UUID")
}

func TestNewMeta_UsesProvidedRequestID(t *testing.T) {
	customID := "my-custom-request-id"

	meta := response.NewMeta(customID)

	assert.Equal(t, customID, meta.RequestID)
}

func TestNewMeta_TimestampIsRFC3339(t *testing.T) {
	before := time.Now().UTC().Add(-1 * time.Second)

	meta := response.NewMeta("")

	parsed, err := time.Parse(time.RFC3339, meta.Timestamp)
	require.NoError(t, err, "timestamp should be valid RFC3339")
	assert.True(t, parsed.After(before) || parsed.Equal(before),
		"timestamp should be recent")
	assert.True(t, parsed.Before(time.Now().UTC().Add(1*time.Second)),
		"timestamp should not be in the future")
}

func TestSuccess_WritesCorrectEnvelope(t *testing.T) {
	// Arrange
	w := httptest.NewRecorder()
	data := map[string]string{"key": "value"}
	requestID := "test-req-id"

	// Act
	response.Success(w, http.StatusOK, data, requestID)

	// Assert
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err)

	assert.NotNil(t, env["data"])
	assert.Nil(t, env["error"])
	assert.NotNil(t, env["meta"])

	meta := env["meta"].(map[string]interface{})
	assert.Equal(t, requestID, meta["requestId"])
	assert.NotEmpty(t, meta["timestamp"])
}

func TestSuccess_Status201(t *testing.T) {
	w := httptest.NewRecorder()

	response.Success(w, http.StatusCreated, "created", "req-1")

	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestErr_WritesErrorEnvelope(t *testing.T) {
	// Arrange
	w := httptest.NewRecorder()
	requestID := "err-req-id"

	// Act
	response.Err(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid input", requestID)

	// Assert
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err)

	assert.Nil(t, env["data"])
	assert.NotNil(t, env["error"])

	apiErr := env["error"].(map[string]interface{})
	assert.Equal(t, "VALIDATION_ERROR", apiErr["code"])
	assert.Equal(t, "invalid input", apiErr["message"])

	meta := env["meta"].(map[string]interface{})
	assert.Equal(t, requestID, meta["requestId"])
}

func TestErrWithDetails_IncludesDetails(t *testing.T) {
	// Arrange
	w := httptest.NewRecorder()
	details := map[string]string{"field": "email", "reason": "required"}

	// Act
	response.ErrWithDetails(w, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "validation failed", details, "det-req")

	// Assert
	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err)

	apiErr := env["error"].(map[string]interface{})
	assert.Equal(t, "VALIDATION_ERROR", apiErr["code"])
	assert.Equal(t, "validation failed", apiErr["message"])
	assert.NotNil(t, apiErr["details"])

	det := apiErr["details"].(map[string]interface{})
	assert.Equal(t, "email", det["field"])
	assert.Equal(t, "required", det["reason"])
}

func TestJSON_SetsContentTypeAndStatus(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"200 OK", http.StatusOK},
		{"201 Created", http.StatusCreated},
		{"400 Bad Request", http.StatusBadRequest},
		{"404 Not Found", http.StatusNotFound},
		{"500 Internal Server Error", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			env := response.Envelope{
				Data:  nil,
				Error: nil,
				Meta:  response.NewMeta(""),
			}

			response.JSON(w, tt.status, env)

			assert.Equal(t, tt.status, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		})
	}
}

func TestErr_NilDataOnError(t *testing.T) {
	w := httptest.NewRecorder()

	response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "something broke", "")

	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err)
	assert.Nil(t, env["data"])
}

func TestSuccess_NilErrorOnSuccess(t *testing.T) {
	w := httptest.NewRecorder()

	response.Success(w, http.StatusOK, "ok", "")

	var env map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &env)
	require.NoError(t, err)
	assert.Nil(t, env["error"])
}
