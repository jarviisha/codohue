package admin

import "net/http"

// CORSMiddleware returns chi-compatible middleware that allows credentialed
// cross-origin requests from exactly the configured origin. Used so the Vite
// dev server at http://localhost:5173 can talk to cmd/admin at
// http://localhost:2002 with session cookies (EventSource needs
// withCredentials, fetch needs credentials: 'include').
//
// When origin is empty the middleware is a no-op — that is the production
// path where the SPA is embedded same-origin in cmd/admin.
func CORSMiddleware(origin string) func(http.Handler) http.Handler {
	if origin == "" {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Origin") == origin {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Add("Vary", "Origin")
				if r.Method == http.MethodOptions {
					h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					h.Set("Access-Control-Allow-Headers", "Content-Type, Cookie, Last-Event-ID")
					h.Set("Access-Control-Max-Age", "600")
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
