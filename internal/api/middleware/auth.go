package middleware

import (
	"context"
	"errors"
	"net/http"

	"github.com/daap14/daap/internal/api/response"
	"github.com/daap14/daap/internal/auth"
)

const identityKey contextKey = "identity"

// Auth is middleware that extracts the X-API-Key header and resolves it
// to an Identity via the auth service. Missing or invalid keys return 401.
func Auth(authService *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := GetRequestID(r.Context())

			rawKey := r.Header.Get("X-API-Key")
			if rawKey == "" {
				response.Err(w, http.StatusUnauthorized, "UNAUTHORIZED", "API key is required", requestID)
				return
			}

			identity, err := authService.Authenticate(r.Context(), rawKey)
			if err != nil {
				if errors.Is(err, auth.ErrInvalidKey) {
					response.Err(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid or revoked API key", requestID)
					return
				}
				response.Err(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Authentication failed", requestID)
				return
			}

			ctx := context.WithValue(r.Context(), identityKey, identity)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetIdentity retrieves the authenticated Identity from the request context.
func GetIdentity(ctx context.Context) *auth.Identity {
	if id, ok := ctx.Value(identityKey).(*auth.Identity); ok {
		return id
	}
	return nil
}
