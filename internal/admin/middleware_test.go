package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func bearerTestHandler(t *testing.T, adminKey string) http.Handler {
	t.Helper()
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	return RequireSessionOrBearer(nil, adminKey)(next)
}

func TestRequireSessionOrBearer_ValidBearerPasses(t *testing.T) {
	h := bearerTestHandler(t, "secret-key")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/admin/v1/overview", http.NoBody)
	req.Header.Set("Authorization", "Bearer secret-key")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("valid bearer must pass, got %d", rec.Code)
	}
}

func TestRequireSessionOrBearer_InvalidBearer401(t *testing.T) {
	h := bearerTestHandler(t, "secret-key")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid bearer must 401, got %d", rec.Code)
	}
}

func TestRequireSessionOrBearer_BearerWinsOverCookie(t *testing.T) {
	// A bearer header is authoritative: an invalid token fails even when a
	// cookie is also present — deterministic precedence, never a silent
	// fallback to the other credential.
	h := bearerTestHandler(t, "secret-key")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer wrong")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "some-session"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("invalid bearer must not fall back to the cookie, got %d", rec.Code)
	}
}

func TestRequireSessionOrBearer_FailedAttemptsThrottled(t *testing.T) {
	h := bearerTestHandler(t, "secret-key")
	var lastCode int
	for range loginBurst + 1 {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)
		req.RemoteAddr = "10.1.2.3:5555"
		req.Header.Set("Authorization", "Bearer wrong")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		lastCode = rec.Code
	}
	if lastCode != http.StatusTooManyRequests {
		t.Fatalf("exhausted budget must 429, got %d", lastCode)
	}

	// The correct key from the same IP is never throttled — the budget only
	// meters failures.
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)
	req.RemoteAddr = "10.9.9.9:1234"
	req.Header.Set("Authorization", "Bearer secret-key")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("correct key must pass, got %d", rec.Code)
	}
}

func TestRequireSessionOrBearer_EmptyKeyDisablesBearerPath(t *testing.T) {
	h := bearerTestHandler(t, "")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("empty configured key must never match, got %d", rec.Code)
	}
}

func TestRequireSessionOrBearer_NoBearerFallsBackToSession(t *testing.T) {
	h := bearerTestHandler(t, "secret-key")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no credentials must 401 via the session path, got %d", rec.Code)
	}
}
