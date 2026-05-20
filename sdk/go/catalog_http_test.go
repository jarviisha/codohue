package codohue

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

func TestNamespaceIngestCatalog(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/v1/namespaces/feed/catalog" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var body codohuetypes.CatalogIngestRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.ObjectID != "post-123" {
			t.Errorf("object_id = %q", body.ObjectID)
		}
		if body.Content != "hello world" {
			t.Errorf("content = %q", body.Content)
		}
		w.WriteHeader(http.StatusAccepted)
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, _ := New(srv.URL)
	err := c.Namespace("feed", "k").IngestCatalog(context.Background(), codohuetypes.CatalogIngestRequest{
		ObjectID: "post-123",
		Content:  "hello world",
		Metadata: map[string]any{"author": "alice"},
	})
	if err != nil {
		t.Fatalf("IngestCatalog: %v", err)
	}
}

func TestNamespaceIngestCatalogMissingObjectID(t *testing.T) {
	t.Parallel()

	c, _ := New("http://unreachable")
	err := c.Namespace("feed", "k").IngestCatalog(context.Background(), codohuetypes.CatalogIngestRequest{
		Content: "no object id",
	})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
}
