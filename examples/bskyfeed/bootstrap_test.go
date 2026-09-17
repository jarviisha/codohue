package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The feeder authenticates with a service token, not a console session: a
// bearer credential cannot create one, so a login step would fail the whole
// bootstrap before it ever reached the upsert.
func TestBootstrapUpsertsWithBearerToken(t *testing.T) {
	var gotAuth, gotMethod, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotMethod, gotPath = r.Header.Get("Authorization"), r.Method, r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	cfg := config{adminURL: srv.URL, adminKey: "svc-token", namespace: "bluesky", embeddingDim: 256}
	if err := bootstrap(context.Background(), cfg); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if gotAuth != "Bearer svc-token" {
		t.Errorf("Authorization = %q, want bearer service token", gotAuth)
	}
	if gotMethod != http.MethodPut || gotPath != "/api/admin/v1/namespaces/bluesky" {
		t.Errorf("%s %s, want PUT /api/admin/v1/namespaces/bluesky", gotMethod, gotPath)
	}
	if gotBody["dense_source"] != "catalog" || gotBody["catalog_strategy_id"] != "internal-hashing-ngrams" {
		t.Errorf("body = %v, want the catalog strategy provisioned in one request", gotBody)
	}
}

// A token without admin:write on this namespace must fail loudly; the feeder
// otherwise publishes into a namespace that does not exist.
func TestBootstrapFailsOnForbidden(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
	}))
	defer srv.Close()

	cfg := config{adminURL: srv.URL, adminKey: "svc-token", namespace: "bluesky", embeddingDim: 256}
	if err := bootstrap(context.Background(), cfg); err == nil {
		t.Fatal("forbidden upsert accepted")
	}
}
