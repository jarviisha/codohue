//go:build e2e

package e2e

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// Public /healthz is reachable by anyone who can reach the port. A raw
// dependency error there leaks the database host, port and often the username
// — the endpoint exists to answer "is this instance serving?", not to describe
// the deployment.
func TestObservability_PublicHealthDisclosesNothing(t *testing.T) {
	for _, target := range []struct{ name, url string }{
		{"api", baseURL + "/healthz"},
		{"embedder", embedderHealthURL + "/healthz"},
	} {
		t.Run(target.name, func(t *testing.T) {
			resp, err := http.Get(target.url) //nolint:noctx // e2e probe against a local listener
			if err != nil {
				t.Skipf("%s listener not running: %v", target.name, err)
			}
			defer resp.Body.Close() //nolint:errcheck

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			text := strings.ToLower(string(body))

			// Component names and dependency hostnames must not appear. The
			// aggregate status still does — that is the whole payload.
			for _, forbidden := range []string{"postgres", "redis", "qdrant", "connection refused", "dial tcp", "5432", "6379", "6334"} {
				if strings.Contains(text, forbidden) {
					t.Errorf("public health leaked %q: %s", forbidden, body)
				}
			}
			if !strings.Contains(text, "status") {
				t.Errorf("public health must still report an aggregate status: %s", body)
			}
		})
	}
}

// details=true is the operator view. It needs the dedicated observability
// credential — and specifically NOT the admin key, because handing a scrape
// agent a metrics token must not hand it the ability to delete namespaces.
func TestObservability_DetailsAndMetricsNeedTheirOwnCredential(t *testing.T) {
	token := os.Getenv("CODOHUE_OBSERVABILITY_TOKEN")
	if token == "" {
		t.Skip("CODOHUE_OBSERVABILITY_TOKEN is not configured for this run")
	}

	for _, target := range []struct{ name, base string }{
		{"api", baseURL},
		{"embedder", embedderHealthURL},
	} {
		for _, path := range []string{"/healthz?details=true", "/metrics"} {
			t.Run(target.name+path, func(t *testing.T) {
				// The embedder only runs for the tests that need it; a dead
				// listener says nothing about the credential split.
				if probe, err := http.Get(target.base + "/healthz"); err != nil { //nolint:noctx // e2e liveness probe
					t.Skipf("%s listener not running: %v", target.name, err)
				} else {
					probe.Body.Close() //nolint:errcheck
				}

				noCredential := doRequest(t, http.MethodGet, target.base+path, "", nil)
				status := noCredential.StatusCode
				noCredential.Body.Close() //nolint:errcheck
				if status != http.StatusUnauthorized {
					t.Errorf("no credential: got %d, want 401", status)
				}

				adminCredential := doRequest(t, http.MethodGet, target.base+path, adminKey, nil)
				status = adminCredential.StatusCode
				adminCredential.Body.Close() //nolint:errcheck
				if status != http.StatusUnauthorized {
					t.Errorf("admin key accepted for observability: got %d, want 401", status)
				}

				trusted := doRequest(t, http.MethodGet, target.base+path, token, nil)
				defer trusted.Body.Close() //nolint:errcheck
				if trusted.StatusCode != http.StatusOK && trusted.StatusCode != http.StatusServiceUnavailable {
					t.Errorf("observability token rejected: got %d", trusted.StatusCode)
				}
			})
		}
	}
}

// A monitoring agent must get real signal, not a sanitized stub: the whole
// point of the separate credential is that the protected view is useful.
func TestObservability_TrustedScrapeSeesComponentDetail(t *testing.T) {
	token := os.Getenv("CODOHUE_OBSERVABILITY_TOKEN")
	if token == "" {
		t.Skip("CODOHUE_OBSERVABILITY_TOKEN is not configured for this run")
	}

	resp := doRequest(t, http.MethodGet, baseURL+"/healthz?details=true", token, nil)
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	for _, want := range []string{"postgres", "redis", "qdrant"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("detailed health is missing component %q: %s", want, body)
		}
	}

	metrics := doRequest(t, http.MethodGet, baseURL+"/metrics", token, nil)
	defer metrics.Body.Close() //nolint:errcheck
	scraped, err := io.ReadAll(metrics.Body)
	if err != nil {
		t.Fatalf("read metrics: %v", err)
	}
	if !strings.Contains(string(scraped), "codohue_") {
		t.Errorf("metrics scrape returned no codohue series: %.200s", scraped)
	}
}

// A bad Authorization header on plain /healthz must be ignored: the liveness
// probe never sends one, and rejecting it would take a healthy instance out of
// rotation the moment a proxy started attaching a token.
func TestObservability_BadHeaderDoesNotBreakLiveness(t *testing.T) {
	resp := doRequest(t, http.MethodGet, baseURL+"/healthz", "nonsense-token", nil)
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusUnauthorized {
		t.Fatal("plain health rejected a request carrying a bad credential")
	}
}
