//go:build e2e

package e2e

import (
	"net/http"
	"testing"
)

func TestPing(t *testing.T) {
	resp := doRequest(t, http.MethodGet, baseURL+"/ping", "", nil)

	var body struct {
		Status string `json:"status"`
	}
	decodeJSON(t, resp, &body)

	if body.Status != "ok" {
		t.Errorf("status = %q, want %q", body.Status, "ok")
	}
}

func TestPingNoAuth(t *testing.T) {
	// /ping requires no authentication.
	resp := doRequest(t, http.MethodGet, baseURL+"/ping", "unexpected-token", nil)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}
