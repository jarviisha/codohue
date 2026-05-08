//go:build e2e

package e2e

import (
	"net/http"
	"testing"
	"time"
)

func TestRecommendComputed_WarmSubjectExcludesSeenItems(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "recommend_computed", map[string]any{
		"action_weights":  map[string]float64{"VIEW": 1.0, "LIKE": 4.0},
		"lambda":          0.01,
		"gamma":           0.5,
		"max_results":     10,
		"seen_items_days": 30,
		"dense_strategy":  "disabled",
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
	seedEvent(t, namespace, "user_b", "item_3", "VIEW", 1.0, now.Add(-18*time.Minute), nil)
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

	resp := doRequest(t, http.MethodGet,
		baseURL+"/v1/namespaces/"+namespace+"/subjects/user_a/recommendations?limit=3",
		apiKey, nil)

	var body struct {
		SubjectID string `json:"subject_id"`
		Namespace string `json:"namespace"`
		Items     []struct {
			ObjectID string `json:"object_id"`
		} `json:"items"`
		Source string `json:"source"`
	}
	decodeJSON(t, resp, &body)

	if body.SubjectID != "user_a" {
		t.Fatalf("subject_id = %q, want user_a", body.SubjectID)
	}
	if body.Namespace != namespace {
		t.Fatalf("namespace = %q, want %q", body.Namespace, namespace)
	}
	if body.Source != "collaborative_filtering" {
		t.Fatalf("source = %q, want collaborative_filtering", body.Source)
	}
	if len(body.Items) == 0 {
		t.Fatal("expected non-empty recommendations for warm subject")
	}
	for _, item := range body.Items {
		if seen[item.ObjectID] {
			t.Fatalf("recommended seen item %q", item.ObjectID)
		}
	}
}

func TestRecommendComputed_ColdStartFallsBackToTrendingOrPopular(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "recommend_cold", map[string]any{
		"action_weights":  map[string]float64{"VIEW": 1.0, "LIKE": 4.0},
		"lambda":          0.01,
		"gamma":           0.5,
		"max_results":     10,
		"seen_items_days": 30,
		"dense_strategy":  "disabled",
		"trending_window": 24,
		"trending_ttl":    120,
	})

	now := time.Now().UTC().Truncate(time.Second)
	seedEvent(t, namespace, "user_b", "item_hot", "LIKE", 4.0, now.Add(-25*time.Minute), nil)
	seedEvent(t, namespace, "user_c", "item_hot", "LIKE", 4.0, now.Add(-20*time.Minute), nil)
	seedEvent(t, namespace, "user_d", "item_warm", "VIEW", 1.0, now.Add(-10*time.Minute), nil)

	runCronOnceUntil(t, 20*time.Second, func() (bool, error) {
		card, ttl := trendingKeyState(t, namespace)
		return card > 0 && ttl > 0, nil
	})

	resp := doRequest(t, http.MethodGet,
		baseURL+"/v1/namespaces/"+namespace+"/subjects/cold_subject/recommendations?limit=3",
		apiKey, nil)

	var body struct {
		Items []struct {
			ObjectID string `json:"object_id"`
		} `json:"items"`
		Source string `json:"source"`
	}
	decodeJSON(t, resp, &body)

	if body.Source != "fallback_popular" {
		t.Fatalf("source = %q, want fallback_popular", body.Source)
	}
	if len(body.Items) == 0 {
		t.Fatal("expected non-empty fallback recommendations for cold subject")
	}
}
