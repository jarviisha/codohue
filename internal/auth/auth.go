package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
	"sync"
	"time"

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

// ConstantTimeEqual compares two strings in constant time. Both sides are
// hashed first so the comparison cost is independent of length and content —
// a plain == short-circuits on the first differing byte, which leaks how much
// of a guessed credential matched.
func ConstantTimeEqual(a, b string) bool {
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

// ValidateNamespaceKey returns true if the token is authorized for the given namespace.
//
//   - When the namespace has a provisioned key, ONLY that key is accepted —
//     the global admin key is deliberately rejected so provisioning a
//     namespace key actually narrows the blast radius of an admin-key leak.
//   - When the namespace has no key, the global admin key is the fallback.
//   - When the hash lookup fails, the request is denied: authorization that
//     cannot be established is not granted.
func ValidateNamespaceKey(ctx context.Context, token, adminKey string, getHash KeyHashFn, namespace string) bool {
	if token == "" {
		return false
	}

	hash, err := getHash(ctx, namespace)
	if err != nil {
		return false
	}
	if hash == "" {
		return adminKey != "" && ConstantTimeEqual(token, adminKey)
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(token)) == nil
}

// negativeCacheTTL bounds how long a rejected (token, namespace) pair is
// remembered. Long enough to blunt a brute-force loop's bcrypt cost, short
// enough that a just-rotated key becomes usable almost immediately.
const negativeCacheTTL = 30 * time.Second

// negativeCache remembers recently rejected credentials so repeated garbage
// tokens don't each cost a namespace_configs lookup plus a ~60ms bcrypt
// compare — otherwise an unauthenticated sender has a cheap CPU-exhaustion
// lever on the data plane. Only failures are cached; successes always
// revalidate.
type negativeCache struct {
	mu      sync.Mutex
	entries map[[32]byte]time.Time // digest -> expiry
	now     func() time.Time
}

func newNegativeCache() *negativeCache {
	return &negativeCache{entries: make(map[[32]byte]time.Time), now: time.Now}
}

func negKey(token, namespace string) [32]byte {
	return sha256.Sum256([]byte(namespace + "\x00" + token))
}

func (c *negativeCache) hit(token, namespace string) bool {
	key := negKey(token, namespace)
	c.mu.Lock()
	defer c.mu.Unlock()
	exp, ok := c.entries[key]
	if !ok {
		return false
	}
	if c.now().After(exp) {
		delete(c.entries, key)
		return false
	}
	return true
}

func (c *negativeCache) put(token, namespace string) {
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	// Opportunistic prune keeps the map bounded by the attack rate within
	// one TTL window rather than growing for the process lifetime.
	for k, exp := range c.entries {
		if now.After(exp) {
			delete(c.entries, k)
		}
	}
	c.entries[negKey(token, namespace)] = now.Add(negativeCacheTTL)
}

// RequireNamespace returns middleware that validates a namespace-scoped API key.
// extractNamespace is called to obtain the namespace from the incoming request
// (e.g. from a URL path parameter or query string).
func RequireNamespace(adminKey string, getHash KeyHashFn, extractNamespace func(*http.Request) string) func(http.Handler) http.Handler {
	neg := newNegativeCache()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := ExtractBearerToken(r)
			namespace := extractNamespace(r)

			if namespace == "" {
				// Let the handler return the appropriate 400; auth cannot validate without a namespace.
				next.ServeHTTP(w, r)
				return
			}

			if neg.hit(token, namespace) {
				httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid or missing bearer token")
				return
			}
			if !ValidateNamespaceKey(r.Context(), token, adminKey, getHash, namespace) {
				neg.put(token, namespace)
				httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid or missing bearer token")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
