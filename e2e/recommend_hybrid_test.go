//go:build e2e

package e2e

import (
	"net/http"
	"testing"
	"time"
)

func TestRecommendHybrid_UsesSparseAndDenseArtifacts(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "recommend_hybrid", map[string]any{
		"action_weights":  map[string]float64{"VIEW": 1.0, "LIKE": 4.0},
		"lambda":          0.01,
		"gamma":           0.2,
		"max_results":     10,
		"seen_items_days": 30,
		"dense_strategy":  "byoe",
		"embedding_dim":   4,
		"alpha":           0.5,
		"dense_distance":  "cosine",
	})

	now := time.Now().UTC().Truncate(time.Second)
	seen := map[string]bool{"item_1": true, "item_2": true}

	seedEvent(t, namespace, "user_a", "item_1", "VIEW", 1.0, now.Add(-50*time.Minute), nil)
	seedEvent(t, namespace, "user_a", "item_2", "LIKE", 4.0, now.Add(-45*time.Minute), nil)
	seedEvent(t, namespace, "user_a", "item_1", "VIEW", 1.0, now.Add(-40*time.Minute), nil)
	seedEvent(t, namespace, "user_a", "item_2", "LIKE", 4.0, now.Add(-35*time.Minute), nil)
	seedEvent(t, namespace, "user_a", "item_2", "LIKE", 4.0, now.Add(-30*time.Minute), nil)

	seedEvent(t, namespace, "user_b", "item_2", "LIKE", 4.0, now.Add(-25*time.Minute), nil)
	seedEvent(t, namespace, "user_b", "item_3", "LIKE", 4.0, now.Add(-20*time.Minute), nil)
	seedEvent(t, namespace, "user_c", "item_2", "VIEW", 1.0, now.Add(-15*time.Minute), nil)
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

	resp := doRequest(t, http.MethodPut, baseURL+"/v1/namespaces/"+namespace+"/objects/item_3/embedding", apiKey, map[string]any{
		"vector": []float32{0.95, 0.05, 0, 0},
	})
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	resp = doRequest(t, http.MethodPut, baseURL+"/v1/namespaces/"+namespace+"/objects/item_4/embedding", apiKey, map[string]any{
		"vector": []float32{0.90, 0.10, 0, 0},
	})
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	resp = doRequest(t, http.MethodPut, baseURL+"/v1/namespaces/"+namespace+"/subjects/user_a/embedding", apiKey, map[string]any{
		"vector": []float32{1, 0, 0, 0},
	})
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()

	resp = doRequest(t, http.MethodGet,
		baseURL+"/v1/namespaces/"+namespace+"/subjects/user_a/recommendations?limit=3",
		apiKey, nil)

	var body struct {
		Items []struct {
			ObjectID string `json:"object_id"`
		} `json:"items"`
		Source string `json:"source"`
	}
	decodeJSON(t, resp, &body)

	if body.Source != "hybrid" {
		t.Fatalf("source = %q, want hybrid", body.Source)
	}
	if len(body.Items) == 0 {
		t.Fatal("expected non-empty hybrid recommendations")
	}
	for _, item := range body.Items {
		if seen[item.ObjectID] {
			t.Fatalf("hybrid recommended seen item %q", item.ObjectID)
		}
	}
}
