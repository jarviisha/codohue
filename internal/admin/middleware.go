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

// RequireSessionOrBearer authenticates either the browser session cookie or
// `Authorization: Bearer <admin key>` — the path automation uses so it does
// not have to impersonate a browser (exchange the key for a cookie first).
//
// A bearer header, when present, is authoritative: it is validated and the
// cookie is ignored, so a stale cookie next to a valid key (or vice versa)
// behaves deterministically. Failed bearer attempts consume the same
// failed-only per-IP budget the login endpoint uses — a correct key is never
// throttled, and this route would otherwise be un-throttled brute-force
// surface. An empty configured key disables the bearer path entirely rather
// than matching empty tokens.
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
				if limiter.Blocked(ip) {
					httpapi.WriteError(w, http.StatusTooManyRequests, "rate_limited", "too many failed attempts; retry later")
					return
				}
				if adminKey == "" || !constantTimeEqual(token, adminKey) {
					limiter.RecordFailure(ip)
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
