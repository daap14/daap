package middleware

import (
	"net/http"

	"github.com/daap14/daap/internal/api/response"
)

// RequireSuperuser returns middleware that rejects non-superuser identities with 403.
func RequireSuperuser() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := GetRequestID(r.Context())

			identity := GetIdentity(r.Context())
			if identity == nil {
				response.Err(w, http.StatusUnauthorized, "UNAUTHORIZED", "API key is required", requestID)
				return
			}

			if !identity.IsSuperuser {
				response.Err(w, http.StatusForbidden, "FORBIDDEN", "Superuser access required", requestID)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireRole returns middleware that rejects identities whose team role is not
// in the allowed list. The superuser (who has no role) is also rejected.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	allowed := make(map[string]bool, len(roles))
	for _, r := range roles {
		allowed[r] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := GetRequestID(r.Context())

			identity := GetIdentity(r.Context())
			if identity == nil {
				response.Err(w, http.StatusUnauthorized, "UNAUTHORIZED", "API key is required", requestID)
				return
			}

			if identity.Role == nil || !allowed[*identity.Role] {
				response.Err(w, http.StatusForbidden, "FORBIDDEN", "Insufficient permissions", requestID)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
