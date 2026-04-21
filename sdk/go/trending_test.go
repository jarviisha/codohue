package codohue

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

func TestNamespaceTrending(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/namespaces/feed/trending" {
			t.Errorf("path = %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("limit") != "25" {
			t.Errorf("limit = %q", q.Get("limit"))
		}
		if q.Get("offset") != "10" {
			t.Errorf("offset = %q", q.Get("offset"))
		}
		if q.Get("window_hours") != "48" {
			t.Errorf("window_hours = %q", q.Get("window_hours"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(codohuetypes.TrendingResponse{
			Namespace:   "feed",
			Items:       []codohuetypes.TrendingItem{{ObjectID: "x", Score: 1.0}},
			WindowHours: 48,
		})
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, _ := New(srv.URL)
	resp, err := c.Namespace("feed", "k").Trending(context.Background(),
		WithLimit(25),
		WithOffset(10),
		WithWindowHours(48),
	)
	if err != nil {
		t.Fatalf("Trending: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].ObjectID != "x" {
		t.Errorf("unexpected: %+v", resp)
	}
}

func TestNamespaceTrendingOmitsZeroOptions(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for _, k := range []string{"limit", "offset", "window_hours"} {
			if _, ok := q[k]; ok {
				t.Errorf("expected %s to be omitted, got %q", k, q.Get(k))
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"namespace":"feed","items":[],"window_hours":0,"generated_at":"2026-04-21T00:00:00Z"}`))
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, _ := New(srv.URL)
	if _, err := c.Namespace("feed", "k").Trending(context.Background()); err != nil {
		t.Fatalf("Trending: %v", err)
	}
}
