//go:build e2e

package e2e

import (
	"net/http"
	"testing"
	"time"
)

func TestTrending_EmptyReturnsEmptyList(t *testing.T) {
	resp := doRequest(t, http.MethodGet,
		baseURL+"/v1/trending/"+testNS, nsKey, nil)

	var body struct {
		Namespace   string    `json:"namespace"`
		Items       []any     `json:"items"`
		WindowHours int       `json:"window_hours"`
		GeneratedAt time.Time `json:"generated_at"`
	}
	decodeJSON(t, resp, &body)

	if body.Namespace != testNS {
		t.Errorf("namespace = %q, want %q", body.Namespace, testNS)
	}
	if body.GeneratedAt.IsZero() {
		t.Error("generated_at is zero")
	}
	if body.Items == nil {
		t.Error("items must not be null (expected empty array)")
	}
}

func TestTrending_Unauthorized(t *testing.T) {
	resp := doRequest(t, http.MethodGet,
		baseURL+"/v1/trending/"+testNS, "wrong-key", nil)
	assertStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()
}

func TestTrending_NoToken(t *testing.T) {
	resp := doRequest(t, http.MethodGet,
		baseURL+"/v1/trending/"+testNS, "", nil)
	assertStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()
}

func TestTrending_InvalidLimit(t *testing.T) {
	resp := doRequest(t, http.MethodGet,
		baseURL+"/v1/trending/"+testNS+"?limit=0", nsKey, nil)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestTrending_NegativeLimit(t *testing.T) {
	resp := doRequest(t, http.MethodGet,
		baseURL+"/v1/trending/"+testNS+"?limit=-1", nsKey, nil)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestTrending_InvalidOffset(t *testing.T) {
	resp := doRequest(t, http.MethodGet,
		baseURL+"/v1/trending/"+testNS+"?offset=-1", nsKey, nil)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestTrending_PaginationParams(t *testing.T) {
	resp := doRequest(t, http.MethodGet,
		baseURL+"/v1/trending/"+testNS+"?limit=5&offset=0", nsKey, nil)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close()
}

func TestTrending_WindowHours(t *testing.T) {
	resp := doRequest(t, http.MethodGet,
		baseURL+"/v1/trending/"+testNS+"?window_hours=48", nsKey, nil)

	var body struct {
		WindowHours int `json:"window_hours"`
	}
	decodeJSON(t, resp, &body)

	if body.WindowHours != 48 {
		t.Errorf("window_hours = %d, want 48", body.WindowHours)
	}
}
