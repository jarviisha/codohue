package admin

import (
	"net/http"
	"strings"

	"github.com/jarviisha/codohue/internal/core/httpapi"
)

const sessionCookieName = "codohue_admin_session"

// RequireSession is middleware that validates the session cookie on every
// request via the SessionManager (signature, expiry, revocation). Missing or
// invalid sessions get 401.
func RequireSession(sessions *SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(sessionCookieName)
			if err != nil || sessions == nil || !sessions.Validate(cookie.Value) {
				httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid or missing session")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireSessionOrBearer is the isolated-handler authentication adapter.
// Production uses Handler.RequireIdentity and the shared identity store.
// Attempts are reserved before checking a legacy key, including matching guesses.
func RequireSessionOrBearer(sessions *SessionManager, adminKey string) func(http.Handler) http.Handler {
	limiter := newLoginRateLimiter()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if header := r.Header.Get("Authorization"); header != "" {
				token, ok := strings.CutPrefix(header, "Bearer ")
				if !ok {
					httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "malformed authorization header")
					return
				}
				ip := clientIP(r)
				if !limiter.Allow(ip) {
					httpapi.WriteError(w, 429, "rate_limited", "too many attempts")
					return
				}
				if adminKey == "" || !constantTimeEqual(token, adminKey) {
					httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid bearer token")
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			cookie, err := r.Cookie(sessionCookieName)
			if err != nil || sessions == nil || !sessions.Validate(cookie.Value) {
				httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid or missing session")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
