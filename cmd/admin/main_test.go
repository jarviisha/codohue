package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jarviisha/codohue/internal/admin"
)

// TestNewAdminRouter_RegistersCatalogRoutes is a smoke test asserting that
// every catalog admin route from contracts/rest-api.md is wired by
// newAdminRouter. Each request omits the session cookie, so the protected
// routes return 401 (which proves the route exists; a 404 would indicate the
// path is unknown to chi). The auth-bypassed POST /api/v1/auth/sessions
// route is asserted with a 400 on missing body.
func TestNewAdminRouter_RegistersCatalogRoutes(t *testing.T) {
	apiKey := "test-key"
	r := newAdminRouter(admin.NewHandler(nil, apiKey), apiKey)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/admin/v1/namespaces/ns/catalog"},
		{http.MethodPut, "/api/admin/v1/namespaces/ns/catalog"},
		{http.MethodPost, "/api/admin/v1/namespaces/ns/catalog/re-embed"},
		{http.MethodGet, "/api/admin/v1/namespaces/ns/catalog/items"},
		{http.MethodGet, "/api/admin/v1/namespaces/ns/catalog/items/1"},
		{http.MethodPost, "/api/admin/v1/namespaces/ns/catalog/items/1/redrive"},
		{http.MethodPost, "/api/admin/v1/namespaces/ns/catalog/items/redrive-deadletter"},
		{http.MethodDelete, "/api/admin/v1/namespaces/ns/catalog/items/1"},
	}

	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, http.NoBody)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code == http.StatusNotFound {
				t.Fatalf("route not registered: %s %s returned 404", tc.method, tc.path)
			}
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("expected 401 (route exists, no session), got %d for %s %s",
					rec.Code, tc.method, tc.path)
			}
		})
	}
}

func TestNewAdminRouter_AuthEndpointReachable(t *testing.T) {
	apiKey := "test-key"
	r := newAdminRouter(admin.NewHandler(nil, apiKey), apiKey)

	// Empty body — handler returns 400 invalid_request, proving the route is wired.
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/auth/sessions", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 invalid_request from /api/v1/auth/sessions, got %d", rec.Code)
	}
}

// TestNewAdminRouter_BulkRedriveBeforeIDRoute guards against a chi routing
// regression: the bulk redrive endpoint shares the same prefix as the {id}
// path, so it must be registered first to avoid 'redrive-deadletter' being
// parsed as the {id} URL parameter.
func TestNewAdminRouter_BulkRedriveBeforeIDRoute(t *testing.T) {
	apiKey := "test-key"
	r := newAdminRouter(admin.NewHandler(nil, apiKey), apiKey)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/admin/v1/namespaces/ns/catalog/items/redrive-deadletter", http.NoBody)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	// Route must exist and require auth (401, not 404).
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bulk redrive route mis-routed; got %d, want 401", rec.Code)
	}
}
