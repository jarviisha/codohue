package codohue

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

func TestNamespaceRecommend(t *testing.T) {
	t.Parallel()

	want := codohuetypes.Response{
		SubjectID:   "user-1",
		Namespace:   "feed",
		Items:       []string{"a", "b", "c"},
		Source:      "collaborative_filtering",
		GeneratedAt: time.Date(2026, 4, 21, 10, 0, 0, 0, time.UTC),
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/v1/namespaces/feed/recommendations" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("subject_id"); got != "user-1" {
			t.Errorf("subject_id = %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "5" {
			t.Errorf("limit = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer ns-key" {
			t.Errorf("auth = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(want)
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ns := c.Namespace("feed", "ns-key")

	got, err := ns.Recommend(context.Background(), "user-1", WithLimit(5))
	if err != nil {
		t.Fatalf("Recommend: %v", err)
	}
	if got.SubjectID != want.SubjectID || got.Source != want.Source || len(got.Items) != len(want.Items) {
		t.Errorf("response = %+v, want %+v", got, want)
	}
}

func TestNamespaceRecommendOmitsLimitWhenZero(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.URL.Query()["limit"]; ok {
			t.Errorf("expected no limit query param, got %q", r.URL.Query().Get("limit"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"subject_id":"u","namespace":"feed","items":[],"source":"x","generated_at":"2026-04-21T00:00:00Z"}`))
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, _ := New(srv.URL)
	if _, err := c.Namespace("feed", "k").Recommend(context.Background(), "u"); err != nil {
		t.Fatalf("Recommend: %v", err)
	}
}
