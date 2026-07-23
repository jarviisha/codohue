package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jarviisha/codohue/internal/core/httpapi"
	"golang.org/x/crypto/bcrypt"
)

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
	}{
		{"valid bearer", "Bearer abc123", "abc123"},
		{"missing header", "", ""},
		{"wrong scheme", "Basic abc123", ""},
		{"bearer only no token", "Bearer ", ""},
		{"bearer with spaces in token", "Bearer my-token-value", "my-token-value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)
			if tt.header != "" {
				r.Header.Set("Authorization", tt.header)
			}
			got := ExtractBearerToken(r)
			if got != tt.want {
				t.Errorf("ExtractBearerToken = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateNamespaceKey(t *testing.T) {
	plaintext := "test-secret-key"
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("generate hash: %v", err)
	}

	getHashWith := func(h string) KeyHashFn {
		return func(_ context.Context, _ string) (string, error) {
			return h, nil
		}
	}

	adminKey := "admin-key"

	t.Run("valid namespace key", func(t *testing.T) {
		ok := ValidateNamespaceKey(context.Background(), plaintext, adminKey, getHashWith(string(hash)), "ns")
		if !ok {
			t.Error("expected valid namespace key to be accepted")
		}
	})

	t.Run("wrong token against namespace key", func(t *testing.T) {
		ok := ValidateNamespaceKey(context.Background(), "wrong-token", adminKey, getHashWith(string(hash)), "ns")
		if ok {
			t.Error("expected wrong token to be rejected")
		}
	})

	t.Run("admin key rejected when namespace has its own key", func(t *testing.T) {
		// Provisioning a namespace key must narrow the blast radius of an
		// admin-key leak — the fallback only applies while no key exists.
		ok := ValidateNamespaceKey(context.Background(), adminKey, adminKey, getHashWith(string(hash)), "ns")
		if ok {
			t.Error("expected admin key to be rejected once the namespace has its own key")
		}
	})

	t.Run("admin key accepted when namespace has no key", func(t *testing.T) {
		ok := ValidateNamespaceKey(context.Background(), adminKey, adminKey, getHashWith(""), "ns")
		if !ok {
			t.Error("expected admin key to be accepted as fallback when namespace has no key")
		}
	})

	t.Run("empty token always rejected", func(t *testing.T) {
		ok := ValidateNamespaceKey(context.Background(), "", adminKey, getHashWith(""), "ns")
		if ok {
			t.Error("expected empty token to be rejected")
		}
	})

	t.Run("getHash error rejects token", func(t *testing.T) {
		ok := ValidateNamespaceKey(context.Background(), plaintext, adminKey, func(_ context.Context, _ string) (string, error) {
			return "", context.DeadlineExceeded
		}, "ns")
		if ok {
			t.Error("expected token to be rejected when hash lookup fails")
		}
	})
}

func TestRequireNamespace(t *testing.T) {
	adminKey := "admin-secret"
	namespaceToken := "ns-token"
	hash, err := bcrypt.GenerateFromPassword([]byte(namespaceToken), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("generate hash: %v", err)
	}

	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	t.Run("missing namespace falls through", func(t *testing.T) {
		nextCalled = false
		handler := RequireNamespace(adminKey, func(_ context.Context, _ string) (string, error) {
			return "", nil
		}, func(_ *http.Request) string { return "" })(next)

		r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if !nextCalled {
			t.Fatal("expected next handler to be called")
		}
	})

	t.Run("invalid token returns unauthorized", func(t *testing.T) {
		nextCalled = false
		handler := RequireNamespace(adminKey, func(_ context.Context, _ string) (string, error) {
			return string(hash), nil
		}, func(_ *http.Request) string { return "ns" })(next)

		r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)
		r.Header.Set("Authorization", "Bearer wrong-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
		var got httpapi.ErrorResponse
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if got.Error.Code != "unauthorized" {
			t.Fatalf("unexpected error code: %+v", got)
		}
		if nextCalled {
			t.Fatal("did not expect next handler to be called")
		}
	})

	t.Run("valid namespace token passes through", func(t *testing.T) {
		nextCalled = false
		handler := RequireNamespace(adminKey, func(_ context.Context, _ string) (string, error) {
			return string(hash), nil
		}, func(_ *http.Request) string { return "ns" })(next)

		r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)
		r.Header.Set("Authorization", "Bearer "+namespaceToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if !nextCalled {
			t.Fatal("expected next handler to be called")
		}
	})

	t.Run("admin fallback works when namespace key is absent", func(t *testing.T) {
		nextCalled = false
		handler := RequireNamespace(adminKey, func(_ context.Context, _ string) (string, error) {
			return "", nil
		}, func(_ *http.Request) string { return "ns" })(next)

		r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)
		r.Header.Set("Authorization", "Bearer "+adminKey)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if !nextCalled {
			t.Fatal("expected next handler to be called")
		}
	})
}

func TestValidateNamespaceKey_HashLookupErrorDeniesEveryone(t *testing.T) {
	failing := func(_ context.Context, _ string) (string, error) {
		return "", errors.New("db down")
	}
	if ValidateNamespaceKey(context.Background(), "admin-key", "admin-key", failing, "ns") {
		t.Fatal("authorization that cannot be established must not be granted")
	}
}

func TestConstantTimeEqual(t *testing.T) {
	if !ConstantTimeEqual("secret", "secret") {
		t.Error("equal strings must match")
	}
	if ConstantTimeEqual("secret", "secreT") || ConstantTimeEqual("secret", "") {
		t.Error("unequal strings must not match")
	}
}

// A repeated bad token must not pay the namespace lookup + bcrypt cost every
// time — that is a cheap CPU-exhaustion lever for an unauthenticated sender.
func TestRequireNamespace_NegativeCacheShortCircuits(t *testing.T) {
	lookups := 0
	getHash := func(_ context.Context, _ string) (string, error) {
		lookups++
		return "", nil // no ns key; token won't match the admin key either
	}
	mw := RequireNamespace("admin-key", getHash, func(*http.Request) string { return "ns" })
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))

	for i := 0; i < 3; i++ {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/namespaces/ns/trending", http.NoBody)
		req.Header.Set("Authorization", "Bearer wrong-token")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status %d, want 401", i, rec.Code)
		}
	}
	if lookups != 1 {
		t.Fatalf("expected 1 hash lookup (then cache hits), got %d", lookups)
	}
}

func TestNegativeCache_ExpiresEntries(t *testing.T) {
	c := newNegativeCache()
	now := time.Now()
	c.now = func() time.Time { return now }

	c.put("tok", "ns")
	if !c.hit("tok", "ns") {
		t.Fatal("fresh entry must hit")
	}
	now = now.Add(negativeCacheTTL + time.Second)
	if c.hit("tok", "ns") {
		t.Fatal("expired entry must miss — a rotated key has to become usable")
	}
}

// An infra error during the hash lookup must NOT negatively cache the token:
// a correct key presented during a DB blip would otherwise be rejected for
// the full TTL after the DB recovers.
func TestRequireNamespace_TransientHashErrorNotCached(t *testing.T) {
	callN := 0
	getHash := func(_ context.Context, _ string) (string, error) {
		callN++
		if callN == 1 {
			return "", errors.New("db blip")
		}
		return "", nil // recovered: no ns key, so the admin key is the fallback
	}
	mw := RequireNamespace("admin-key", getHash, func(*http.Request) string { return "ns" })
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))

	// First request during the blip → 401, but must NOT be cached.
	req1 := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/namespaces/ns/trending", http.NoBody)
	req1.Header.Set("Authorization", "Bearer admin-key")
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusUnauthorized {
		t.Fatalf("during blip: got %d, want 401", rec1.Code)
	}

	// Second request after recovery with the SAME (correct) token must pass —
	// a cached negative would have rejected it.
	req2 := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/namespaces/ns/trending", http.NoBody)
	req2.Header.Set("Authorization", "Bearer admin-key")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("after recovery: got %d, want 200 — a transient error must not be negatively cached", rec2.Code)
	}
}
