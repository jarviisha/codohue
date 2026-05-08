package codohue

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

func TestNamespaceRank(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/v1/namespaces/feed/rankings" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("content-type = %q", got)
		}

		var body codohuetypes.RankRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		// Namespace is in the path, not in the body.
		if body.SubjectID != "user-1" {
			t.Errorf("subject_id = %q", body.SubjectID)
		}
		if len(body.Candidates) != 2 {
			t.Errorf("candidates = %v", body.Candidates)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(codohuetypes.RankResponse{
			SubjectID: "user-1",
			Namespace: "feed",
			Items: []codohuetypes.RankedItem{
				{ObjectID: "a", Score: 0.9},
				{ObjectID: "b", Score: 0.1},
			},
			Source: "hybrid_rank",
		})
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, _ := New(srv.URL)
	resp, err := c.Namespace("feed", "k").Rank(context.Background(), "user-1", []string{"a", "b"})
	if err != nil {
		t.Fatalf("Rank: %v", err)
	}
	if len(resp.Items) != 2 || resp.Items[0].ObjectID != "a" {
		t.Errorf("unexpected response: %+v", resp)
	}
}
