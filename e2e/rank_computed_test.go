//go:build e2e

package e2e

import (
	"net/http"
	"testing"
	"time"
)

func TestRankComputed_WarmSubjectRanksConnectedCandidateFirst(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "rank_computed", map[string]any{
		"action_weights":  map[string]float64{"VIEW": 1.0, "LIKE": 4.0},
		"lambda":          0.01,
		"gamma":           0.5,
		"max_results":     10,
		"seen_items_days": 30,
		"dense_strategy":  "disabled",
	})

	now := time.Now().UTC().Truncate(time.Second)

	seedEvent(t, namespace, "user_a", "item_1", "VIEW", 1.0, now.Add(-50*time.Minute), nil)
	seedEvent(t, namespace, "user_a", "item_2", "LIKE", 4.0, now.Add(-45*time.Minute), nil)
	seedEvent(t, namespace, "user_a", "item_1", "VIEW", 1.0, now.Add(-40*time.Minute), nil)
	seedEvent(t, namespace, "user_a", "item_2", "LIKE", 4.0, now.Add(-35*time.Minute), nil)
	seedEvent(t, namespace, "user_a", "item_2", "LIKE", 4.0, now.Add(-30*time.Minute), nil)

	seedEvent(t, namespace, "user_b", "item_2", "LIKE", 4.0, now.Add(-25*time.Minute), nil)
	seedEvent(t, namespace, "user_b", "item_3", "LIKE", 4.0, now.Add(-20*time.Minute), nil)
	seedEvent(t, namespace, "user_b", "item_3", "LIKE", 4.0, now.Add(-18*time.Minute), nil)

	seedEvent(t, namespace, "user_c", "item_5", "VIEW", 1.0, now.Add(-15*time.Minute), nil)
	seedEvent(t, namespace, "user_c", "item_4", "VIEW", 1.0, now.Add(-10*time.Minute), nil)

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

	resp := doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+namespace+"/rankings", apiKey, map[string]any{
		"subject_id": "user_a",
		"candidates": []string{"item_4", "item_3"},
	})

	var body struct {
		Items []struct {
			ObjectID string  `json:"object_id"`
			Score    float64 `json:"score"`
		} `json:"items"`
		Source string `json:"source"`
	}
	decodeJSON(t, resp, &body)

	if body.Source != "hybrid_rank" {
		t.Fatalf("source = %q, want hybrid_rank", body.Source)
	}
	if len(body.Items) == 0 {
		t.Fatal("expected at least one ranked candidate")
	}
	if body.Items[0].ObjectID != "item_3" {
		t.Fatalf("top ranked item = %q, want item_3", body.Items[0].ObjectID)
	}
	if body.Items[0].Score <= 0 {
		t.Fatalf("top score = %.4f, want positive score", body.Items[0].Score)
	}
}
