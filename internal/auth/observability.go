package auth

import (
	"net/http"

	"github.com/jarviisha/codohue/internal/core/httpapi"
)

// RequireObservability protects metrics and detailed diagnostics with their
// dedicated bearer token. An unset token disables the endpoint as 404; the
// global admin credential is deliberately not part of this contract.
func RequireObservability(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token == "" {
				http.NotFound(w, r)
				return
			}
			provided := ExtractBearerToken(r)
			if provided == "" || !ConstantTimeEqual(provided, token) {
				httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid or missing observability bearer token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
