package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/jarviisha/codohue/internal/core/httpapi"
	"golang.org/x/crypto/bcrypt"
)

// KeyHashFn is a function that retrieves the bcrypt-hashed API key for a namespace.
// Returns an empty string (and nil error) when the namespace exists but has no key configured.
// Returns a non-nil error only on infrastructure failures.
type KeyHashFn func(ctx context.Context, namespace string) (hash string, err error)

// ExtractBearerToken returns the token from the "Authorization: Bearer <token>" header,
// or an empty string if the header is absent or malformed.
func ExtractBearerToken(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if !strings.HasPrefix(v, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(v, "Bearer ")
}

// ValidateNamespaceKey returns true if the token is authorized for the given namespace.
// It accepts the token when:
//   - The namespace has a key and the token matches its bcrypt hash, OR
//   - The namespace has no key configured and the token matches the admin key (fallback
//     for clients not yet provisioned with a namespace key).
func ValidateNamespaceKey(ctx context.Context, token, adminKey string, getHash KeyHashFn, namespace string) bool {
	if token == "" {
		return false
	}

	hash, err := getHash(ctx, namespace)
	if err != nil {
		return false
	}

	if hash != "" {
		return bcrypt.CompareHashAndPassword([]byte(hash), []byte(token)) == nil
	}

	// Namespace has no key yet — accept admin key as fallback.
	return token == adminKey
}

// RequireAdmin returns middleware that allows only requests carrying the global admin key.
func RequireAdmin(adminKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if ExtractBearerToken(r) != adminKey {
				httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid or missing bearer token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireNamespace returns middleware that validates a namespace-scoped API key.
// extractNamespace is called to obtain the namespace from the incoming request
// (e.g. from a URL path parameter or query string).
func RequireNamespace(adminKey string, getHash KeyHashFn, extractNamespace func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := ExtractBearerToken(r)
			namespace := extractNamespace(r)

			if namespace == "" {
				// Let the handler return the appropriate 400; auth cannot validate without a namespace.
				next.ServeHTTP(w, r)
				return
			}

			if !ValidateNamespaceKey(r.Context(), token, adminKey, getHash, namespace) {
				httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid or missing bearer token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
