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

// The observability credential is deliberately NOT the global admin key. An
// operator who hands a monitoring agent a scrape token must not be handing it
// the ability to delete every namespace, so possession of one must never
// satisfy the other.
func TestRequireObservability_AdminKeyIsNotASubstitute(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	const adminKey = "dev-secret-key"

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+adminKey)
	rec := httptest.NewRecorder()

	RequireObservability("observability-token")(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("admin key was accepted for observability: status=%d", rec.Code)
	}
}

// An unconfigured token means the route does not exist, not that it exists and
// rejects you: a 401 would confirm to an unauthenticated prober that the
// deployment has metrics to find.
func TestRequireObservability_UnconfiguredHidesTheRoute(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

	for _, header := range []string{"", "Bearer anything", "Bearer "} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", http.NoBody)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		rec := httptest.NewRecorder()

		RequireObservability("")(next).ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("header=%q: status=%d, want 404", header, rec.Code)
		}
	}
}

// Near-miss tokens must be rejected whatever their shape — a prefix, a
// suffix, or a same-length neighbour.
func TestRequireObservability_RejectsNearMisses(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	const token = "s3cr3t-observability"

	for _, provided := range []string{
		"s3cr3t-observabilit",   // one short
		"s3cr3t-observabilityy", // one long
		"S3CR3T-OBSERVABILITY",  // case flipped
		" s3cr3t-observability", // leading space
	} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/metrics", http.NoBody)
		req.Header.Set("Authorization", "Bearer "+provided)
		rec := httptest.NewRecorder()

		RequireObservability(token)(next).ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("token %q accepted: status=%d", provided, rec.Code)
		}
	}
}
