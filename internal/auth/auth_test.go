package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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

	t.Run("admin key rejected when namespace has a key", func(t *testing.T) {
		ok := ValidateNamespaceKey(context.Background(), adminKey, adminKey, getHashWith(string(hash)), "ns")
		if ok {
			t.Error("expected admin key to be rejected when namespace has its own key")
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

func TestRequireAdmin(t *testing.T) {
	adminKey := "super-secret"
	handler := RequireAdmin(adminKey)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("valid admin key", func(t *testing.T) {
		r := httptest.NewRequestWithContext(context.Background(), http.MethodPut, "/", http.NoBody)
		r.Header.Set("Authorization", "Bearer "+adminKey)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)
		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", rec.Code)
		}
	})

	t.Run("wrong key", func(t *testing.T) {
		r := httptest.NewRequestWithContext(context.Background(), http.MethodPut, "/", http.NoBody)
		r.Header.Set("Authorization", "Bearer wrong")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
		}
		var got httpapi.ErrorResponse
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("decode error response: %v", err)
		}
		if got.Error.Code != "unauthorized" {
			t.Fatalf("unexpected error code: %+v", got)
		}
	})

	t.Run("missing header", func(t *testing.T) {
		r := httptest.NewRequestWithContext(context.Background(), http.MethodPut, "/", http.NoBody)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, r)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", rec.Code)
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
