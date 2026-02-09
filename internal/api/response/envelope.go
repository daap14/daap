package response

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Meta holds metadata for every API response.
type Meta struct {
	RequestID string `json:"requestId"`
	Timestamp string `json:"timestamp"`
}

// ListMeta extends Meta with pagination information.
type ListMeta struct {
	Meta
	Total int `json:"total"`
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

// Error represents a structured API error.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// Envelope is the standard API response wrapper.
type Envelope struct {
	Data  any    `json:"data"`
	Error *Error `json:"error"`
	Meta  Meta   `json:"meta"`
}

// ListEnvelope is the response wrapper for list endpoints with pagination metadata.
type ListEnvelope struct {
	Data  any      `json:"data"`
	Error *Error   `json:"error"`
	Meta  ListMeta `json:"meta"`
}

// NewMeta creates a Meta with a new UUID and current timestamp.
// If requestID is provided, it uses that instead of generating a new one.
func NewMeta(requestID string) Meta {
	if requestID == "" {
		requestID = uuid.New().String()
	}
	return Meta{
		RequestID: requestID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

// JSON writes a JSON response with the given status code and envelope.
func JSON(w http.ResponseWriter, status int, env Envelope) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(env); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

// Success writes a successful JSON response.
func Success(w http.ResponseWriter, status int, data any, requestID string) {
	JSON(w, status, Envelope{
		Data:  data,
		Error: nil,
		Meta:  NewMeta(requestID),
	})
}

// SuccessList writes a successful list JSON response with pagination metadata.
func SuccessList(w http.ResponseWriter, status int, data any, total, page, limit int, requestID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	env := ListEnvelope{
		Data:  data,
		Error: nil,
		Meta: ListMeta{
			Meta:  NewMeta(requestID),
			Total: total,
			Page:  page,
			Limit: limit,
		},
	}
	if err := json.NewEncoder(w).Encode(env); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

// NoContent writes a 204 No Content response.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// Err writes an error JSON response.
func Err(w http.ResponseWriter, status int, code string, message string, requestID string) {
	JSON(w, status, Envelope{
		Data: nil,
		Error: &Error{
			Code:    code,
			Message: message,
		},
		Meta: NewMeta(requestID),
	})
}

// ErrWithDetails writes an error JSON response with additional details.
func ErrWithDetails(w http.ResponseWriter, status int, code string, message string, details any, requestID string) {
	JSON(w, status, Envelope{
		Data: nil,
		Error: &Error{
			Code:    code,
			Message: message,
			Details: details,
		},
		Meta: NewMeta(requestID),
	})
}
