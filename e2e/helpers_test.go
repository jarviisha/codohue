//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// doRequest fires an HTTP request and returns the response.
// The caller is responsible for closing resp.Body.
// If token is non-empty it is sent as a Bearer token.
// If body is non-nil it is JSON-encoded and Content-Type is set accordingly.
func doRequest(t *testing.T, method, url, token string, body any) *http.Response {
	t.Helper()
	var r io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
		r = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, url, r)
	if err != nil {
		t.Fatalf("new request %s %s: %v", method, url, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request %s %s: %v", method, url, err)
	}
	return resp
}

// assertStatus fails the test if resp.StatusCode != want.
// On failure it reads and prints the response body before calling t.Fatalf.
func assertStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected HTTP %d, got %d: %s", want, resp.StatusCode, bytes.TrimSpace(body))
	}
}

// doRawPost fires a POST request with a raw string body (Content-Type: application/json).
// Use this when you need to send deliberately malformed JSON.
func doRawPost(t *testing.T, url, token, rawBody string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(rawBody))
	if err != nil {
		t.Fatalf("new request POST %s: %v", url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request POST %s: %v", url, err)
	}
	return resp
}

// decodeJSON asserts status 200 and decodes the JSON response body into v.
// It always closes resp.Body.
func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	assertStatus(t, resp, http.StatusOK)
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode JSON response: %v", err)
	}
}
