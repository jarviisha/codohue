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

func TestNamespaceIngestCatalogBatch(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/namespaces/feed/catalog/batch" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var body codohuetypes.CatalogBatchIngestRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(body.Items) != 2 {
			t.Errorf("items = %d", len(body.Items))
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(codohuetypes.CatalogBatchIngestResponse{
			Namespace: "feed", Accepted: 1, Rejected: 1,
			Results: []codohuetypes.CatalogBatchItemResult{
				{ObjectID: "a", Accepted: true},
				{ObjectID: "b", Accepted: false, Error: "empty_content"},
			},
		})
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, _ := New(srv.URL)
	resp, err := c.Namespace("feed", "k").IngestCatalogBatch(context.Background(), []codohuetypes.CatalogIngestRequest{
		{ObjectID: "a", Content: "hello"},
		{ObjectID: "b", Content: " "},
	})
	if err != nil {
		t.Fatalf("IngestCatalogBatch: %v", err)
	}
	if resp.Accepted != 1 || resp.Rejected != 1 || resp.Results[1].Error != "empty_content" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestNamespaceIngestCatalogBatch_ClientSideCap(t *testing.T) {
	t.Parallel()

	c, _ := New("http://localhost:0")
	over := make([]codohuetypes.CatalogIngestRequest, codohuetypes.CatalogBatchMaxItems+1)
	for i := range over {
		over[i] = codohuetypes.CatalogIngestRequest{ObjectID: "o", Content: "c"}
	}
	if _, err := c.Namespace("feed", "k").IngestCatalogBatch(context.Background(), over); err == nil {
		t.Fatal("expected client-side cap error")
	}
	if _, err := c.Namespace("feed", "k").IngestCatalogBatch(context.Background(), nil); err == nil {
		t.Fatal("expected empty-items error")
	}
}

func TestNamespaceListCatalogObjects(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/namespaces/feed/catalog/objects" {
			t.Errorf("path = %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("changed_since") != "2026-01-01T00:00:00Z" || q.Get("limit") != "50" {
			t.Errorf("query = %s", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(codohuetypes.CatalogObjectsResponse{
			Namespace: "feed", Total: 1, Limit: 50,
			Items: []codohuetypes.CatalogObjectSummary{{ObjectID: "a", UpdatedAt: "2026-01-02T03:04:05Z"}},
		})
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, _ := New(srv.URL)
	resp, err := c.Namespace("feed", "k").ListCatalogObjects(context.Background(), "2026-01-01T00:00:00Z", 50, 0)
	if err != nil {
		t.Fatalf("ListCatalogObjects: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].ObjectID != "a" {
		t.Errorf("unexpected response: %+v", resp)
	}
}
