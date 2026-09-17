package admin

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jarviisha/codohue/internal/core/access"
)

type identityFake struct {
	IdentityStore
	actor    access.Actor
	attempts int
	audits   int
}

func (f *identityFake) Session(context.Context, string) (access.Actor, error) { return f.actor, nil }
func (f *identityFake) ServiceToken(context.Context, string) (access.Actor, error) {
	return f.actor, nil
}
func (f *identityFake) Audit(context.Context, string, string, string) error { f.audits++; return nil }
func (f *identityFake) Login(_ context.Context, _, _, password string) (access.Actor, error) {
	f.attempts++
	if f.attempts > 5 {
		return access.Actor{}, access.ErrThrottled
	}
	if password != "correct" {
		return access.Actor{}, access.ErrCredentials
	}
	return f.actor, nil
}
func (f *identityFake) IssueSession(context.Context, access.Actor) (string, time.Time, error) {
	return "opaque", time.Now().Add(time.Hour), nil
}
func TestIdentityAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name, role, path, method string
		perms, nss               []string
		cookie, csrf             bool
		want                     int
	}{
		{name: "consumer-admin", role: "service", path: "/api/admin/v1/namespaces/a", method: "GET", perms: []string{"data:read"}, nss: []string{"a"}, want: 403},
		{name: "scoped-admin", role: "service", path: "/api/admin/v1/namespaces/a", method: "GET", perms: []string{"admin:read"}, nss: []string{"a"}, want: 204},
		{name: "other-namespace", role: "service", path: "/api/admin/v1/namespaces/b", method: "GET", perms: []string{"admin:read"}, nss: []string{"a"}, want: 403},
		{name: "machine-reset", role: "service", path: "/api/admin/v1/reset", method: "POST", perms: []string{"admin:write"}, nss: []string{"*"}, want: 403},
		{name: "admin-account", role: "admin", path: "/api/admin/v1/accounts/other", method: "PUT", cookie: true, csrf: true, want: 403},
		{name: "owner-account", role: "owner", path: "/api/admin/v1/accounts/other", method: "PUT", cookie: true, csrf: true, want: 204},
		{name: "missing-csrf", role: "owner", path: "/api/admin/v1/reset", method: "POST", cookie: true, want: 403},
		{name: "provision-only", role: "service", path: "/api/admin/v1/namespaces/a", method: "PUT", perms: []string{"namespace:provision"}, nss: []string{"a"}, want: 204},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &identityFake{actor: access.Actor{Name: "test", Role: tc.role, Permissions: tc.perms, Namespaces: tc.nss}}
			h := NewHandler(nil, "", nil)
			h.SetIdentity(f, IdentityOptions{})
			r := chi.NewRouter()
			r.Group(func(r chi.Router) {
				r.Use(h.RequireIdentity(nil, ""))
				for _, path := range []string{"/api/admin/v1/namespaces/{ns}", "/api/admin/v1/reset", "/api/admin/v1/accounts/{username}"} {
					r.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
				}
			})
			req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, http.NoBody)
			if tc.cookie {
				req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session"})
			} else {
				req.Header.Set("Authorization", "Bearer service-token")
			}
			if tc.csrf {
				req.Header.Set("X-Codohue-CSRF", "1")
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("got %d want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
		})
	}
}
func TestOperatorLoginThrottlesMatchingCandidate(t *testing.T) {
	f := &identityFake{actor: access.Actor{Name: "owner", Role: "owner"}}
	h := NewHandler(nil, "global-key", nil)
	h.SetIdentity(f, IdentityOptions{})
	for i := 0; i < 6; i++ {
		password := "wrong"
		want := 401
		if i == 5 {
			password = "correct"
			want = 429
		}
		req := httptest.NewRequestWithContext(context.Background(), "POST", "/api/v1/auth/sessions", strings.NewReader(`{"username":"owner","password":"`+password+`"}`))
		req.Header.Set("X-Codohue-CSRF", "1")
		rec := httptest.NewRecorder()
		h.CreateSession(rec, req)
		if rec.Code != want {
			t.Fatalf("attempt %d: %d", i, rec.Code)
		}
	}
}
func TestTrustedProxyAndCSRF(t *testing.T) {
	h := NewHandler(nil, "", nil)
	prefixes, err := ParseTrustedProxies("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	h.identityOptions.TrustedProxies = prefixes
	r := httptest.NewRequestWithContext(context.Background(), "POST", "http://admin.example/api/test", http.NoBody)
	r.RemoteAddr = "192.0.2.1:80"
	r.Header.Set("X-Forwarded-Proto", "https")
	r.Header.Set("X-Forwarded-For", "198.51.100.1")
	if h.secureCookie(r) || h.remoteIP(r) != "192.0.2.1" {
		t.Fatal("untrusted forwarded headers used")
	}
	r.RemoteAddr = "10.1.1.1:80"
	r.Header.Set("X-Forwarded-For", "192.0.2.99, 198.51.100.1, 10.2.2.2")
	if !h.secureCookie(r) || h.remoteIP(r) != "198.51.100.1" {
		t.Fatal("trusted proxy chain incorrect")
	}
	r.Header.Set("X-Codohue-CSRF", "1")
	r.Header.Set("Origin", "https://evil.example")
	if h.csrfOK(r) {
		t.Fatal("foreign origin accepted")
	}
	r.Header.Set("Origin", "https://admin.example")
	if !h.csrfOK(r) {
		t.Fatal("same origin rejected")
	}
}

type streamIdentityFake struct {
	identityFake
	revoked atomic.Bool
}

func (f *streamIdentityFake) Session(context.Context, string) (access.Actor, error) {
	if f.revoked.Load() {
		return access.Actor{}, access.ErrCredentials
	}
	return f.actor, nil
}

func (f *streamIdentityFake) ServiceToken(ctx context.Context, token string) (access.Actor, error) {
	return f.Session(ctx, token)
}

func TestActiveStreamClosesOnRevocation(t *testing.T) {
	for _, service := range []bool{false, true} {
		t.Run(fmt.Sprint("service=", service), func(t *testing.T) {
			actor := access.Actor{Name: "operator", Role: "admin"}
			if service {
				actor = access.Actor{Name: "machine", Role: "service", Permissions: []string{"admin:read"}, Namespaces: []string{"*"}}
			}
			f := &streamIdentityFake{identityFake: identityFake{actor: actor}}
			h := NewHandler(nil, "", nil)
			h.SetIdentity(f, IdentityOptions{})
			h.sessionCheckInterval = time.Millisecond
			started := make(chan struct{})
			closed := make(chan struct{})
			router := chi.NewRouter()
			router.Group(func(r chi.Router) {
				r.Use(h.RequireIdentity(nil, ""))
				r.Get("/api/admin/v1/stream", func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done(); close(closed) })
			})
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			req := httptest.NewRequestWithContext(ctx, "GET", "/api/admin/v1/stream", http.NoBody)
			if service {
				req.Header.Set("Authorization", "Bearer service-token")
			} else {
				req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "session"})
			}
			done := make(chan struct{})
			go func() { defer close(done); router.ServeHTTP(httptest.NewRecorder(), req) }()
			<-started
			f.revoked.Store(true)
			select {
			case <-closed:
				if ctx.Err() != nil {
					t.Fatal("stream only closed at request deadline")
				}
			case <-ctx.Done():
				t.Fatal("revoked credential retained active stream")
			}
			<-done
		})
	}
}
