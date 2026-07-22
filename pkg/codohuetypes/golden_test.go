package codohuetypes_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// update regenerates the golden files instead of asserting against them.
// Run `go test ./pkg/codohuetypes/... -run Golden -update` after a *deliberate*
// wire-contract change, then commit the updated testdata so the diff is part of
// the review.
var update = flag.Bool("update", false, "update .golden.json wire-contract snapshots")

// These snapshots lock the JSON wire contract that both the server (via the
// type aliases in internal/...) and the public SDK marshal/unmarshal. Renaming,
// removing, retyping, or re-tagging any field changes the marshaled output and
// fails the matching test — turning silent contract drift into a reviewed,
// intentional golden update.
func TestGoldenWireContract(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	objCreated := time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC)

	cases := []struct {
		name string
		v    any
	}{
		{"recommended_item", codohuetypes.RecommendedItem{ObjectID: "obj-1", Score: 0.875, Rank: 1}},
		{"response", codohuetypes.Response{
			SubjectID: "subj-1", Namespace: "feed",
			Items:  []codohuetypes.RecommendedItem{{ObjectID: "obj-1", Score: 0.875, Rank: 1}},
			Source: "collaborative_filtering", Limit: 20, Offset: 0, Total: 1, GeneratedAt: ts,
		}},
		{"rank_request", codohuetypes.RankRequest{SubjectID: "subj-1", Candidates: []string{"obj-1", "obj-2"}}},
		{"ranked_item", codohuetypes.RankedItem{ObjectID: "obj-1", Score: 0.5, Rank: 1}},
		{"rank_response", codohuetypes.RankResponse{
			SubjectID: "subj-1", Namespace: "feed",
			Items:  []codohuetypes.RankedItem{{ObjectID: "obj-1", Score: 0.5, Rank: 1}},
			Source: "hybrid_rank", Total: 1, GeneratedAt: ts,
		}},
		{"trending_item", codohuetypes.TrendingItem{ObjectID: "obj-1", Score: 12.5}},
		{"trending_response", codohuetypes.TrendingResponse{
			Namespace:   "feed",
			Items:       []codohuetypes.TrendingItem{{ObjectID: "obj-1", Score: 12.5}},
			WindowHours: 24, Limit: 20, Offset: 0, Total: 1, GeneratedAt: ts,
		}},
		{"embedding_request", codohuetypes.EmbeddingRequest{Vector: []float32{0.1, 0.2, 0.3, 0.4}}},
		{"event_payload", codohuetypes.EventPayload{
			Namespace: "feed", SubjectID: "subj-1", ObjectID: "obj-1", Action: codohuetypes.ActionLike,
			OccurredAt: ts, ObjectCreatedAt: &objCreated, Metadata: map[string]string{"src": "web"},
		}},
		{"catalog_ingest_request", codohuetypes.CatalogIngestRequest{
			ObjectID: "obj-1", Content: "hello world", AuthorSubjectID: "subj-1",
			Metadata: map[string]any{"lang": "en"},
		}},
		{"error_detail", codohuetypes.ErrorDetail{Code: "invalid_request", Message: "invalid request body"}},
		{"error_response", codohuetypes.ErrorResponse{
			Error: codohuetypes.ErrorDetail{Code: "unauthorized", Message: "missing bearer token"},
		}},
	}

	seen := make(map[string]bool, len(cases))
	for _, tc := range cases {
		seen[tc.name] = true
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.MarshalIndent(tc.v, "", "  ")
			if err != nil {
				t.Fatalf("marshal %s: %v", tc.name, err)
			}
			got = append(got, '\n')

			path := filepath.Join("testdata", tc.name+".golden.json")
			if *update {
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatalf("write golden %s: %v", path, err)
				}
				return
			}

			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden %s: %v\nrun: go test ./pkg/codohuetypes/... -run Golden -update", path, err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("wire contract drift for %s.\n--- got ---\n%s\n--- want ---\n%s\n"+
					"If this change is intentional, regenerate: go test ./pkg/codohuetypes/... -run Golden -update",
					tc.name, got, want)
			}
		})
	}

	// Guard against orphaned or forgotten snapshots: every testdata file must
	// map to a case above, and vice versa. A removed type leaves a stale file;
	// a new type added without a case leaves an unmatched file once generated.
	if !*update {
		files, err := filepath.Glob(filepath.Join("testdata", "*.golden.json"))
		if err != nil {
			t.Fatalf("glob testdata: %v", err)
		}
		var orphans []string
		for _, f := range files {
			name := filepath.Base(f)
			name = name[:len(name)-len(".golden.json")]
			if !seen[name] {
				orphans = append(orphans, name)
			}
		}
		sort.Strings(orphans)
		if len(orphans) > 0 {
			t.Errorf("orphaned golden snapshots with no matching case: %v", orphans)
		}
	}
}
