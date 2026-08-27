//go:build e2e

package e2e

import (
	"net/http"
	"os"
	"testing"
)

// /healthz has two surfaces, and the split is the point: the public one is
// reachable by anything that can open a socket, so it reports only whether the
// service is serving. Naming which dependency is down, and why, tells an
// unauthenticated caller where to push — that detail lives behind the
// observability credential on ?details=true.
func TestHealthzPublicSurfaceIsAggregateOnly(t *testing.T) {
	resp := doRequest(t, http.MethodGet, baseURL+"/healthz", "", nil)

	var body map[string]any
	decodeJSON(t, resp, &body)

	status, ok := body["status"].(string)
	if !ok {
		t.Fatalf("healthz response has no string \"status\": %v", body)
	}
	if status != "ok" {
		t.Errorf("overall status = %q, want %q", status, "ok")
	}
	// Anything beyond the aggregate is a leak, whatever it is called.
	for key := range body {
		if key != "status" {
			t.Errorf("public /healthz exposed %q; per-dependency detail belongs behind ?details=true", key)
		}
	}
}

// The detailed surface is what operators actually page on, so it has to carry
// the per-dependency breakdown — and it has to refuse callers without the
// observability credential, otherwise the split above buys nothing.
func TestHealthzDetailedSurfaceRequiresObservabilityToken(t *testing.T) {
	token := os.Getenv("CODOHUE_OBSERVABILITY_TOKEN")
	if token == "" {
		t.Skip("CODOHUE_OBSERVABILITY_TOKEN not configured for this environment")
	}

	unauth := doRequest(t, http.MethodGet, baseURL+"/healthz?details=true", "", nil)
	if unauth.StatusCode != http.StatusUnauthorized {
		t.Errorf("unauthenticated ?details=true got %d, want %d", unauth.StatusCode, http.StatusUnauthorized)
	}
	unauth.Body.Close() //nolint:errcheck

	resp := doRequest(t, http.MethodGet, baseURL+"/healthz?details=true", token, nil)
	var body map[string]string
	decodeJSON(t, resp, &body)

	for _, key := range []string{"status", "postgres", "redis", "qdrant"} {
		got, ok := body[key]
		if !ok {
			t.Errorf("detailed healthz missing key %q", key)
			continue
		}
		if got != "ok" {
			t.Errorf("%s = %q, want %q", key, got, "ok")
		}
	}
}
