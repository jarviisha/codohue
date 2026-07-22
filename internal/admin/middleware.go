package admin

import (
	"net/http"

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
