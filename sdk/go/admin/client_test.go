package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestProvisionCatalogNamespace_OneBearerRequest(t *testing.T) {
	t.Parallel()

	var gotAuth string
	var gotBody map[string]any
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/api/admin/v1/namespaces/feed" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if _, hasCookie := r.Header["Cookie"]; hasCookie {
			t.Error("bearer client must not send cookies")
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"namespace":  "feed",
			"updated_at": time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC),
			"api_key":    "plain-key-once",
		})
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, err := New(srv.URL, "admin-key")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := c.ProvisionCatalogNamespace(context.Background(), "feed", ProvisionCatalogRequest{
		EmbeddingDim: 128,
	})
	if err != nil {
		t.Fatalf("ProvisionCatalogNamespace: %v", err)
	}

	if gotAuth != "Bearer admin-key" {
		t.Errorf("authorization = %q", gotAuth)
	}
	if gotBody["dense_source"] != "catalog" {
		t.Errorf("dense_source = %v", gotBody["dense_source"])
	}
	if gotBody["catalog_strategy_id"] != "internal-hashing-ngrams" || gotBody["catalog_strategy_version"] != "v1" {
		t.Errorf("strategy defaults not applied: %v", gotBody)
	}
	params, _ := gotBody["catalog_strategy_params"].(map[string]any)
	if params["dim"] != float64(128) {
		t.Errorf("strategy params dim = %v", params["dim"])
	}
	if res.APIKey != "plain-key-once" {
		t.Errorf("api key not surfaced: %+v", res)
	}
}

func TestProvisionCatalogNamespace_SurfacesAPIError(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"code": "invalid_config", "message": "strategy dimension mismatch: strategy_dim=64 namespace_embedding_dim=128"},
		})
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, _ := New(srv.URL, "admin-key")
	_, err := c.ProvisionCatalogNamespace(context.Background(), "feed", ProvisionCatalogRequest{EmbeddingDim: 128})
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusBadRequest || apiErr.Code != "invalid_config" {
		t.Errorf("unexpected api error: %+v", apiErr)
	}
}

func TestNew_RequiresKeyAndURL(t *testing.T) {
	t.Parallel()
	if _, err := New("", "k"); err == nil {
		t.Error("expected error for empty URL")
	}
	if _, err := New("http://x", ""); err == nil {
		t.Error("expected error for empty key")
	}
}
