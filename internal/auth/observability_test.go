package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireObservabilityContract(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	for _, tc := range []struct {
		name, configured, header string
		want                     int
	}{
		{"disabled", "", "Bearer obs", http.StatusNotFound},
		{"missing", "obs", "", http.StatusUnauthorized},
		{"malformed", "obs", "Basic obs", http.StatusUnauthorized},
		{"wrong including admin key", "obs", "Bearer admin", http.StatusUnauthorized},
		{"valid", "obs", "Bearer obs", http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", http.NoBody)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			RequireObservability(tc.configured)(next).ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status=%d, want %d", rec.Code, tc.want)
			}
		})
	}
}
