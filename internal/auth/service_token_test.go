package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jarviisha/codohue/internal/core/access"
	"golang.org/x/crypto/bcrypt"
)

func TestRequireNamespaceServicePermissions(t *testing.T) {
	for _, tt := range []struct {
		name, method, path, permission, namespace string
		status                                    int
	}{
		{"read", "GET", "/recommendations", "data:read", "shop", 204},
		{"head", "HEAD", "/recommendations", "data:read", "shop", 204},
		{"ranking reads", "POST", "/rankings", "data:read", "shop", 204},
		{"write", "POST", "/events", "data:write", "shop", 204},
		{"wildcard namespace", "GET", "/recommendations", "data:read", "*", 204},
		{"read cannot write", "POST", "/events", "data:read", "shop", 403},
		{"write cannot read", "GET", "/recommendations", "data:write", "shop", 403},
		{"other namespace", "GET", "/recommendations", "data:read", "other", 403},
		{"admin cannot read data", "GET", "/recommendations", "admin:read", "shop", 403},
	} {
		t.Run(tt.name, func(t *testing.T) {
			lookup := func(_ context.Context, token string) (access.Actor, error) {
				if token != "service-token" {
					t.Fatalf("unexpected token %q", token)
				}
				return access.Actor{Permissions: []string{tt.permission}, Namespaces: []string{tt.namespace}}, nil
			}
			handler := RequireNamespace("", func(context.Context, string) (string, error) {
				t.Fatal("recognized service credentials must not fall back to namespace keys")
				return "", nil
			}, func(*http.Request) string { return "shop" }, lookup)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			r := httptest.NewRequestWithContext(context.Background(), tt.method, tt.path, http.NoBody)
			r.Header.Set("Authorization", "Bearer service-token")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if w.Code != tt.status {
				t.Fatalf("status = %d, want %d: %s", w.Code, tt.status, w.Body.String())
			}
		})
	}
}

func TestRequireNamespaceRevalidatesServiceCredentials(t *testing.T) {
	lookupErr := access.ErrCredentials
	lookupCalls := 0
	handler := RequireNamespace("", func(context.Context, string) (string, error) {
		return "", nil
	}, func(*http.Request) string { return "shop" }, func(context.Context, string) (access.Actor, error) {
		lookupCalls++
		return access.Actor{Permissions: []string{"data:read"}, Namespaces: []string{"shop"}}, lookupErr
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	// Issuing a credential bypasses a previous negative cache entry; revoking it
	// takes effect on the next request, even after successful authentication.
	for _, step := range []struct {
		err    error
		status int
	}{
		{access.ErrCredentials, 401},
		{nil, 204},
		{access.ErrCredentials, 401},
		{context.DeadlineExceeded, 401},
	} {
		lookupErr = step.err
		r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/recommendations", http.NoBody)
		r.Header.Set("Authorization", "Bearer service-token")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != step.status {
			t.Fatalf("status = %d, want %d", w.Code, step.status)
		}
	}
	if lookupCalls != 4 {
		t.Fatalf("service lookup called %d times, want 4", lookupCalls)
	}
}

func TestRequireNamespaceKeyWithServiceLookup(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("namespace-key"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	handler := RequireNamespace("", func(context.Context, string) (string, error) {
		return string(hash), nil
	}, func(*http.Request) string { return "shop" }, func(context.Context, string) (access.Actor, error) {
		return access.Actor{}, access.ErrCredentials
	})(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/events", http.NoBody)
	r.Header.Set("Authorization", "Bearer namespace-key")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("namespace key rejected: %d %s", w.Code, w.Body.String())
	}
}
