//go:build e2e

package e2e

import (
	"net/http"
	"testing"
)

func TestHealthzAllDepsOK(t *testing.T) {
	resp := doRequest(t, http.MethodGet, baseURL+"/healthz", "", nil)

	var body struct {
		Status   string `json:"status"`
		Postgres string `json:"postgres"`
		Redis    string `json:"redis"`
		Qdrant   string `json:"qdrant"`
	}
	decodeJSON(t, resp, &body)

	if body.Status != "ok" {
		t.Errorf("overall status = %q, want %q", body.Status, "ok")
	}
	for dep, got := range map[string]string{
		"postgres": body.Postgres,
		"redis":    body.Redis,
		"qdrant":   body.Qdrant,
	} {
		if got != "ok" {
			t.Errorf("%s = %q, want %q", dep, got, "ok")
		}
	}
}

func TestHealthzResponseShape(t *testing.T) {
	// Verify that all expected keys are present in the response body.
	resp := doRequest(t, http.MethodGet, baseURL+"/healthz", "", nil)

	var body map[string]string
	decodeJSON(t, resp, &body)

	for _, key := range []string{"status", "postgres", "redis", "qdrant"} {
		if _, ok := body[key]; !ok {
			t.Errorf("healthz response missing key %q", key)
		}
	}
}
