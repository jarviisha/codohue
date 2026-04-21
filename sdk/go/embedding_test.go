package codohue

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

func TestStoreObjectEmbedding(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/v1/namespaces/feed/objects/item-42/embedding" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var body codohuetypes.EmbeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(body.Vector) != 3 {
			t.Errorf("vector len = %d", len(body.Vector))
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, _ := New(srv.URL)
	err := c.Namespace("feed", "k").StoreObjectEmbedding(
		context.Background(), "item-42", []float32{0.1, 0.2, 0.3},
	)
	if err != nil {
		t.Fatalf("StoreObjectEmbedding: %v", err)
	}
}

func TestStoreSubjectEmbedding(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/namespaces/feed/subjects/u-1/embedding" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, _ := New(srv.URL)
	err := c.Namespace("feed", "k").StoreSubjectEmbedding(
		context.Background(), "u-1", []float32{1, 2, 3},
	)
	if err != nil {
		t.Fatalf("StoreSubjectEmbedding: %v", err)
	}
}

func TestStoreEmbeddingValidation(t *testing.T) {
	t.Parallel()

	c, _ := New("http://example.test")
	ns := c.Namespace("feed", "k")

	if err := ns.StoreObjectEmbedding(context.Background(), "", []float32{1}); err == nil {
		t.Error("expected error on empty id")
	}
	if err := ns.StoreObjectEmbedding(context.Background(), "x", nil); err == nil {
		t.Error("expected error on empty vector")
	}
}

func TestStoreEmbeddingMapsDimMismatch(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":{"code":"embedding_dimension_mismatch","message":"expected 64 got 32"}}`)
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, _ := New(srv.URL)
	err := c.Namespace("feed", "k").StoreObjectEmbedding(
		context.Background(), "item-1", []float32{0.1},
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrDimMismatch) {
		t.Errorf("errors.Is ErrDimMismatch = false; got %v", err)
	}
}

func TestDeleteObject(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/v1/namespaces/feed/objects/item-9" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, _ := New(srv.URL)
	if err := c.Namespace("feed", "k").DeleteObject(context.Background(), "item-9"); err != nil {
		t.Fatalf("DeleteObject: %v", err)
	}
}

func TestDeleteObjectValidation(t *testing.T) {
	t.Parallel()

	c, _ := New("http://example.test")
	if err := c.Namespace("feed", "k").DeleteObject(context.Background(), ""); err == nil {
		t.Error("expected error on empty id")
	}
}
