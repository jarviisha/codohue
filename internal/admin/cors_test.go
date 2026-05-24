package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func newOKHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestCORSAllowsMatchingOrigin(t *testing.T) {
	h := CORSMiddleware("http://localhost:5173")(newOKHandler())
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("Access-Control-Allow-Origin=%q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Access-Control-Allow-Credentials=%q", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary=%q, want Origin", got)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("code=%d, want 200 (handler ran)", rec.Code)
	}
}

func TestCORSIgnoresNonMatchingOrigin(t *testing.T) {
	h := CORSMiddleware("http://localhost:5173")(newOKHandler())
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin=%q, want empty", got)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("code=%d, want 200", rec.Code)
	}
}

func TestCORSPreflightShortCircuits(t *testing.T) {
	called := false
	h := CORSMiddleware("http://localhost:5173")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	req := httptest.NewRequest("OPTIONS", "/", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if called {
		t.Fatal("preflight should not reach inner handler")
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("code=%d, want 204", rec.Code)
	}
	for _, hdr := range []string{
		"Access-Control-Allow-Methods",
		"Access-Control-Allow-Headers",
		"Access-Control-Max-Age",
	} {
		if rec.Header().Get(hdr) == "" {
			t.Errorf("missing %s on preflight response", hdr)
		}
	}
}

func TestCORSPreflightFromNonMatchingOriginDoesNotShortCircuit(t *testing.T) {
	called := false
	h := CORSMiddleware("http://localhost:5173")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusMethodNotAllowed)
	}))
	req := httptest.NewRequest("OPTIONS", "/", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !called {
		t.Fatal("non-matching origin preflight must fall through to inner handler")
	}
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("code=%d, want 405", rec.Code)
	}
}

func TestCORSEmptyOriginIsNoop(t *testing.T) {
	h := CORSMiddleware("")(newOKHandler())
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("ACAO=%q, want empty (no-op middleware)", got)
	}
}
