package codohue

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// A reconciliation walk follows next_cursor until the server stops sending
// one. The cursor is opaque to the client: it is echoed back verbatim, never
// parsed, so the server can change its shape without breaking callers.
func TestNamespaceListCatalogObjects_FollowsTheServerCursor(t *testing.T) {
	t.Parallel()

	var seen []string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/namespaces/feed/catalog/objects" {
			t.Errorf("path = %s", r.URL.Path)
		}
		seen = append(seen, r.URL.Query().Get("cursor"))
		resp := codohuetypes.CatalogObjectsResponse{Namespace: "feed", Limit: 2, Total: 3}
		switch r.URL.Query().Get("cursor") {
		case "":
			resp.Items = []codohuetypes.CatalogObjectSummary{{ObjectID: "a"}, {ObjectID: "b"}}
			resp.NextCursor = "page-2"
		default:
			resp.Items = []codohuetypes.CatalogObjectSummary{{ObjectID: "c"}}
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, _ := New(srv.URL)
	ns := c.Namespace("feed", "k")

	var walked []string
	cursor := ""
	for {
		page, err := ns.ListCatalogObjectsPage(context.Background(), "", 2, 0, cursor)
		if err != nil {
			t.Fatalf("ListCatalogObjectsPage: %v", err)
		}
		for _, item := range page.Items {
			walked = append(walked, item.ObjectID)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}

	if len(walked) != 3 || walked[0] != "a" || walked[2] != "c" {
		t.Errorf("walk = %v, want [a b c]", walked)
	}
	if len(seen) != 2 || seen[0] != "" || seen[1] != "page-2" {
		t.Errorf("cursor not echoed verbatim: %v", seen)
	}
}

// Offset and cursor are two different paging models over the same ordering.
// Combining them is a caller mistake caught before the request goes out —
// the server would otherwise have to guess which one to honour.
func TestNamespaceListCatalogObjects_CursorAndOffsetAreExclusive(t *testing.T) {
	t.Parallel()

	c, _ := New("http://127.0.0.1:1")
	if _, err := c.Namespace("feed", "k").ListCatalogObjectsPage(context.Background(), "", 10, 20, "page-2"); err == nil {
		t.Fatal("expected combining cursor and offset to be rejected")
	}
}

// The pre-cursor entry point still works: it is the cursor call with an empty
// cursor, so an existing offset-based reconciliation keeps running unchanged.
func TestNamespaceListCatalogObjects_LegacyOffsetStillWorks(t *testing.T) {
	t.Parallel()

	var gotQuery string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode(codohuetypes.CatalogObjectsResponse{Namespace: "feed", Limit: 10, Offset: 20})
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, _ := New(srv.URL)
	if _, err := c.Namespace("feed", "k").ListCatalogObjects(context.Background(), "2026-01-01T00:00:00Z", 10, 20); err != nil {
		t.Fatalf("ListCatalogObjects: %v", err)
	}
	for _, want := range []string{"changed_since=2026-01-01T00%3A00%3A00Z", "limit=10", "offset=20"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query %q missing %q", gotQuery, want)
		}
	}
	if strings.Contains(gotQuery, "cursor=") {
		t.Errorf("legacy call must not send a cursor: %q", gotQuery)
	}
}
