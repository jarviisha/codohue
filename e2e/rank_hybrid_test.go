//go:build e2e

package e2e

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// TestRankHybrid_BlendsSparseAndDenseSignals is the US1 end-to-end case: a
// namespace with dense vectors ranks a candidate list with differentiated,
// non-zero scores — a candidate reachable only through the dense side (no
// co-interaction path) must still outscore an unrelated one.
func TestRankHybrid_BlendsSparseAndDenseSignals(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "rank_hybrid", map[string]any{
		"action_weights":  map[string]float64{"VIEW": 1.0, "LIKE": 4.0},
		"lambda":          0.01,
		"gamma":           0.02,
		"max_results":     10,
		"seen_items_days": 30,
		"dense_source":    "byoe",
		"embedding_dim":   4,
		"alpha":           0.5,
		"dense_distance":  "cosine",
	})

	now := time.Now().UTC().Truncate(time.Second)

	// user_a's own history: item_1, item_2.
	seedEvent(t, namespace, "user_a", "item_1", "VIEW", 1.0, now.Add(-50*time.Minute), nil)
	seedEvent(t, namespace, "user_a", "item_2", "LIKE", 4.0, now.Add(-45*time.Minute), nil)
	seedEvent(t, namespace, "user_a", "item_2", "LIKE", 4.0, now.Add(-40*time.Minute), nil)
	// user_b bridges item_2 → item_3, giving item_3 a sparse CF path.
	seedEvent(t, namespace, "user_b", "item_2", "LIKE", 4.0, now.Add(-35*time.Minute), nil)
	seedEvent(t, namespace, "user_b", "item_3", "LIKE", 4.0, now.Add(-30*time.Minute), nil)
	// item_4 and item_5 have no interaction history at all — only dense
	// vectors can differentiate them.
	seedEvent(t, namespace, "user_c", "item_1", "VIEW", 1.0, now.Add(-25*time.Minute), nil)

	runCronOnceUntil(t, 20*time.Second, func() (bool, error) {
		if !qdrantCollectionExists(t, namespace+"_subjects") {
			return false, nil
		}
		if !qdrantCollectionExists(t, namespace+"_objects") {
			return false, nil
		}
		if qdrantPointCount(t, namespace+"_subjects") == 0 {
			return false, nil
		}
		if qdrantPointCount(t, namespace+"_objects") == 0 {
			return false, nil
		}
		return true, nil
	})

	for objectID, vec := range map[string][]float32{
		"item_4": {0.95, 0.05, 0, 0}, // close to user_a's subject vector
		"item_5": {0, 0, 0, 1},       // orthogonal — dense says irrelevant
	} {
		resp := doRequest(t, http.MethodPut, baseURL+"/v1/namespaces/"+namespace+"/objects/"+objectID+"/embedding", apiKey, map[string]any{
			"vector": vec,
		})
		assertStatus(t, resp, http.StatusNoContent)
		resp.Body.Close()
	}
	resp := doRequest(t, http.MethodPut, baseURL+"/v1/namespaces/"+namespace+"/subjects/user_a/embedding", apiKey, map[string]any{
		"vector": []float32{1, 0, 0, 0},
	})
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	resp = doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+namespace+"/rankings", apiKey, map[string]any{
		"subject_id": "user_a",
		"candidates": []string{"item_5", "item_4", "item_3"},
	})
	assertStatus(t, resp, http.StatusOK)
	defer resp.Body.Close()

	var body struct {
		Items []struct {
			ObjectID string  `json:"object_id"`
			Score    float64 `json:"score"`
		} `json:"items"`
		Source string `json:"source"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode rankings response: %v", err)
	}
	if body.Source != "hybrid_rank" {
		t.Fatalf("source: got %q want hybrid_rank", body.Source)
	}
	if len(body.Items) != 3 {
		t.Fatalf("items: got %d want 3 (%+v)", len(body.Items), body.Items)
	}

	scores := map[string]float64{}
	for _, it := range body.Items {
		scores[it.ObjectID] = it.Score
	}
	if scores["item_4"] <= 0 {
		t.Fatalf("dense-only candidate item_4 must score > 0, got %f (all: %v)", scores["item_4"], scores)
	}
	if scores["item_4"] <= scores["item_5"] {
		t.Fatalf("dense-similar item_4 (%f) must outscore orthogonal item_5 (%f)", scores["item_4"], scores["item_5"])
	}
	if scores["item_3"] <= scores["item_5"] {
		t.Fatalf("sparse-connected item_3 (%f) must outscore unrelated item_5 (%f)", scores["item_3"], scores["item_5"])
	}
}
