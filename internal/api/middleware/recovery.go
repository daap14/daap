package middleware

import (
	"log/slog"
	"net/http"

	"github.com/daap14/daap/internal/api/response"
)

// Recovery is middleware that recovers from panics and returns a 500 error.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				requestID := GetRequestID(r.Context())
				slog.Error("panic recovered", "error", err, "requestId", requestID)
				response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", requestID)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
