package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestDecodeStrict_ValidBody(t *testing.T) {
	var out struct {
		SubjectID string `json:"subject_id"`
	}
	if err := DecodeStrict(strings.NewReader(`{"subject_id":"u1"}`), &out); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.SubjectID != "u1" {
		t.Fatalf("subject_id = %q, want u1", out.SubjectID)
	}
}

func TestDecodeStrict_RejectsUnknownField(t *testing.T) {
	var out struct {
		SubjectID string `json:"subject_id"`
	}
	err := DecodeStrict(strings.NewReader(`{"subject_id":"u1","namespace":"x"}`), &out)
	if err == nil {
		t.Fatal("expected an error for an unknown field, got nil")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("error = %q, want it to mention the unknown field", err)
	}
}

func TestDecodeStrict_RejectsTrailingData(t *testing.T) {
	var out struct {
		SubjectID string `json:"subject_id"`
	}
	err := DecodeStrict(strings.NewReader(`{"subject_id":"u1"}{"subject_id":"u2"}`), &out)
	if err == nil {
		t.Fatal("expected an error for trailing data, got nil")
	}
	if !strings.Contains(err.Error(), "trailing data") {
		t.Fatalf("error = %q, want it to mention trailing data", err)
	}
}

func TestDecodeStrict_RejectsMalformedJSON(t *testing.T) {
	var out struct {
		SubjectID string `json:"subject_id"`
	}
	if err := DecodeStrict(strings.NewReader(`not-json`), &out); err == nil {
		t.Fatal("expected an error for malformed JSON, got nil")
	}
}

func TestDecodeStrictMax_RejectsOversizedBody(t *testing.T) {
	var out struct {
		Value string `json:"value"`
	}
	err := DecodeStrictMax(strings.NewReader(`{"value":"123456789"}`), &out, 8)
	if !errors.Is(err, ErrBodyTooLarge) {
		t.Fatalf("expected ErrBodyTooLarge, got %v", err)
	}
}

// TestURLParam_DecodesThroughRealRouter pins the behaviour the helper exists
// for: chi matches on r.URL.RawPath, so an escaped reserved character reaches
// chi.URLParam still escaped. Only a real mux reproduces this — a hand-built
// RouteContext stores whatever the test put in it.
func TestURLParam_DecodesThroughRealRouter(t *testing.T) {
	cases := []struct {
		target string
		want   string
	}{
		{"/ns/bsky/subjects/did:plc:abc/x", "did:plc:abc"},
		{"/ns/bsky/subjects/did%3Aplc%3Aabc/x", "did:plc:abc"},
		{"/ns/bsky/subjects/at%3A%2F%2Fdid%3Aplc%3Aabc%2Fpost%2F1/x", "at://did:plc:abc/post/1"},

		// Decode exactly once. chi returns the already-decoded segment when
		// RawPath is empty, which is the case for an id whose own text
		// contains a '%': net/http resolved "%25" to "%" and the escaped and
		// default spellings then agree. Unescaping again would eat the '%'
		// and resolve to a different id than the client named.
		{"/ns/bsky/subjects/a%252Fb/x", "a%2Fb"},
		{"/ns/bsky/subjects/100%2525/x", "100%25"},
		{"/ns/bsky/subjects/q%3Fs%3D%2520/x", "q?s=%20"},
	}

	for _, tc := range cases {
		var got string
		r := chi.NewRouter()
		r.Get("/ns/{ns}/subjects/{id}/x", func(_ http.ResponseWriter, req *http.Request) {
			got = URLParam(req, "id")
		})
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(context.Background(), http.MethodGet, tc.target, http.NoBody))
		if got != tc.want {
			t.Errorf("GET %s: id = %q, want %q", tc.target, got, tc.want)
		}
	}
}

// TestURLParam_MalformedEscapePassesThrough covers the decode-failure branch.
// net/url only records a RawPath whose escapes all parse, so reaching this
// needs both set by hand; the guard keeps the raw text rather than silently
// yielding "" and tripping a missing-parameter 400.
func TestURLParam_MalformedEscapePassesThrough(t *testing.T) {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "100%")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/x", http.NoBody)
	req.URL.RawPath = "/100%"
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	if got := URLParam(req, "id"); got != "100%" {
		t.Errorf("id = %q, want %q", got, "100%")
	}
}

// TestURLParam_NoRawPathReturnsChiValueVerbatim pins the other half of the
// guard: with no RawPath there is nothing left to decode, and a hand-populated
// RouteContext (what most handler tests use) must pass straight through.
func TestURLParam_NoRawPathReturnsChiValueVerbatim(t *testing.T) {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "a%2Fb")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/x", http.NoBody)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	if got := URLParam(req, "id"); got != "a%2Fb" {
		t.Errorf("id = %q, want %q", got, "a%2Fb")
	}
}
