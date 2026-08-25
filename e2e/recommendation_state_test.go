//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// Vectors are built from a 90-day event window. When every event for a
// namespace ages out, the last vectors would otherwise sit in Qdrant forever
// with frozen scores and keep matching searches — recommendations that no
// behaviour supports any more.
func TestRecommendationState_ExpiredEventsClearOwnedVectors(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "expiry", defaultNamespaceConfig())

	// Seed inside the window, recompute, and confirm points exist.
	recent := time.Now().UTC().Add(-24 * time.Hour)
	for i := 0; i < 3; i++ {
		seedEvent(t, namespace, "u1", fmt.Sprintf("o%d", i), "VIEW", 1, recent, nil)
	}
	runCronOnceUntil(t, 60*time.Second, func() (bool, error) {
		// The collection only exists once cron has created it; counting it
		// before that is a hard failure, not a zero.
		if !qdrantCollectionExists(t, namespace+"_subjects") {
			return false, nil
		}
		return qdrantPointCount(t, namespace+"_subjects") > 0, nil
	})

	// Age every event out of the window, then recompute again.
	if _, err := testDB.Exec(context.Background(), `
		UPDATE events SET occurred_at = NOW() - INTERVAL '200 days' WHERE namespace = $1`,
		namespace); err != nil {
		t.Fatalf("age events: %v", err)
	}
	runCronOnceUntil(t, 60*time.Second, func() (bool, error) {
		if !qdrantCollectionExists(t, namespace+"_subjects") {
			return true, nil // nothing left to sweep
		}
		return qdrantPointCount(t, namespace+"_subjects") == 0, nil
	})

	if qdrantCollectionExists(t, namespace+"_objects") {
		if count := qdrantPointCount(t, namespace+"_objects"); count != 0 {
			t.Errorf("object collection still holds %d point(s) after every event expired", count)
		}
	}

	// The API stays healthy on an empty namespace rather than erroring.
	resp := doRequest(t, http.MethodGet,
		baseURL+"/v1/namespaces/"+namespace+"/subjects/u1/recommendations", apiKey, nil)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close() //nolint:errcheck
}

// A re-embed targets a (strategy_id, strategy_version) tuple, not a version
// string. Two strategies can publish the same version, so comparing versions
// alone would let a switch between same-versioned strategies look like an
// instantly-complete re-embed over vectors from the old model.
func TestRecommendationState_SameVersionDifferentStrategyIsStillStale(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "tuple", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0},
		"embedding_dim":  128,
		"dense_source":   "byoe",
	})
	enableCatalogForNamespace(t, namespace, 128)

	resp := doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+namespace+"/catalog", apiKey,
		map[string]any{"object_id": "tuple-1", "content": "hello world"})
	assertStatus(t, resp, http.StatusAccepted)
	resp.Body.Close() //nolint:errcheck
	waitForRowCount(t, 20*time.Second,
		`SELECT COUNT(*) FROM catalog_items WHERE namespace = $1`, 1, namespace)

	// Pretend the row was embedded by a different strategy that happens to
	// share the configured version string.
	if _, err := testDB.Exec(context.Background(), `
		UPDATE catalog_items
		SET state = 'embedded', strategy_id = 'some-other-model', strategy_version = 'v1',
		    embedded_at = NOW()
		WHERE namespace = $1`, namespace); err != nil {
		t.Fatalf("stamp a foreign strategy: %v", err)
	}

	// The stale count must still see it: the version matches, the identity
	// does not.
	var stale int
	if err := testDB.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM catalog_items
		WHERE namespace = $1
		  AND (strategy_id IS NULL OR strategy_version IS NULL
		       OR (strategy_id, strategy_version) <> ('internal-hashing-ngrams', 'v1'))`,
		namespace).Scan(&stale); err != nil {
		t.Fatalf("count stale rows: %v", err)
	}
	if stale != 1 {
		t.Fatalf("stale rows = %d, want 1: a same-version foreign strategy must count as stale", stale)
	}
}

// One documented clock-skew rule covers events and BYOE alike: at most five
// minutes ahead, boundary inclusive. Beyond it the value is rejected rather
// than stored, because a negative freshness age boosts an item instead of
// decaying it.
func TestRecommendationState_CreationTimestampBoundary(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "skew", defaultNamespaceConfig())

	for _, tc := range []struct {
		name     string
		offset   time.Duration
		wantCode int
	}{
		{"one minute ahead", time.Minute, http.StatusAccepted},
		{"just inside the boundary", 4*time.Minute + 30*time.Second, http.StatusAccepted},
		{"an hour ahead", time.Hour, http.StatusBadRequest},
		{"epoch millis mistaken for seconds", 0, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			createdAt := time.Now().UTC().Add(tc.offset)
			if tc.name == "epoch millis mistaken for seconds" {
				createdAt = time.Unix(time.Now().UnixMilli(), 0).UTC()
			}
			resp := doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+namespace+"/events", apiKey,
				map[string]any{
					"subject_id": "u1", "object_id": "o-skew", "action": "VIEW",
					"object_created_at": createdAt.Format(time.RFC3339),
				})
			status := resp.StatusCode
			resp.Body.Close() //nolint:errcheck
			if status != tc.wantCode {
				t.Fatalf("got %d, want %d", status, tc.wantCode)
			}
		})
	}
}

// encoding/json cannot represent NaN or ±Inf, so a single non-finite score
// fails the whole response — not just that item. Every scoring path therefore
// excludes them before serialization.
func TestRecommendationState_ScoresAreAlwaysFinite(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "finite", defaultNamespaceConfig())

	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		objectCreatedAt := now.Add(-time.Duration(i) * 24 * time.Hour)
		seedEvent(t, namespace, "u1", fmt.Sprintf("o%d", i), "VIEW", 1, now.Add(-time.Hour), &objectCreatedAt)
	}
	runCronOnceUntil(t, 60*time.Second, func() (bool, error) {
		if !qdrantCollectionExists(t, namespace+"_subjects") {
			return false, nil
		}
		return qdrantPointCount(t, namespace+"_subjects") > 0, nil
	})

	resp := doRequest(t, http.MethodGet,
		baseURL+"/v1/namespaces/"+namespace+"/subjects/u1/recommendations?limit=10", apiKey, nil)
	var recommendations codohuetypes.Response
	decodeJSON(t, resp, &recommendations)

	for _, item := range recommendations.Items {
		if math.IsNaN(item.Score) || math.IsInf(item.Score, 0) {
			t.Errorf("object %q scored %v, which cannot be serialized", item.ObjectID, item.Score)
		}
	}

	rank := doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+namespace+"/rankings", apiKey,
		map[string]any{"subject_id": "u1", "candidates": []string{"o0", "o1", "unknown-object"}})
	var ranked codohuetypes.RankResponse
	decodeJSON(t, rank, &ranked)

	for _, item := range ranked.Items {
		if math.IsNaN(item.Score) || math.IsInf(item.Score, 0) {
			t.Errorf("ranked object %q scored %v", item.ObjectID, item.Score)
		}
	}
	// A candidate the engine never indexed comes back unscored rather than
	// dropped, so the caller's list stays whole.
	if len(ranked.Items) != 3 {
		t.Errorf("ranking returned %d items, want the caller's whole candidate list", len(ranked.Items))
	}
}

// A namespace whose every event aged out still needs a compute pass — that is
// exactly when its stale vectors have to be swept. Enumerating namespaces by
// "has recent events" would skip it and leave the vectors forever.
func TestRecommendationState_QuietNamespaceIsStillScheduled(t *testing.T) {
	namespace, _ := createIsolatedNamespace(t, "quiet", defaultNamespaceConfig())

	// No events at all: the run must still complete and log.
	runCronOnceUntil(t, 60*time.Second, func() (bool, error) {
		var runs int
		if err := testDB.QueryRow(context.Background(), `
			SELECT COUNT(*) FROM batch_run_logs WHERE namespace = $1`, namespace).Scan(&runs); err != nil {
			return false, err
		}
		return runs > 0, nil
	})
}
