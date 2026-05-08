//go:build e2e

package e2e

import (
	"net/http"
	"testing"
	"time"
)

func TestCronRecompute_BuildsSparseDenseAndTrendingArtifacts(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "cron_recompute", map[string]any{
		"action_weights":  map[string]float64{"VIEW": 1.0, "LIKE": 4.0},
		"lambda":          0.01,
		"gamma":           0.5,
		"max_results":     20,
		"dense_strategy":  "svd",
		"embedding_dim":   2,
		"alpha":           0.7,
		"dense_distance":  "cosine",
		"trending_window": 24,
		"trending_ttl":    120,
		"lambda_trending": 0.1,
		"seen_items_days": 30,
	})

	now := time.Now().UTC().Truncate(time.Second)
	createdAt := now.Add(-2 * time.Hour)

	seedEvent(t, namespace, "user_a", "item_1", "VIEW", 1.0, now.Add(-30*time.Minute), &createdAt)
	seedEvent(t, namespace, "user_a", "item_2", "LIKE", 4.0, now.Add(-20*time.Minute), &createdAt)
	seedEvent(t, namespace, "user_b", "item_2", "LIKE", 4.0, now.Add(-10*time.Minute), &createdAt)
	seedEvent(t, namespace, "user_b", "item_3", "VIEW", 1.0, now.Add(-5*time.Minute), &createdAt)

	runCronOnceUntil(t, 20*time.Second, func() (bool, error) {
		if !qdrantCollectionExists(t, namespace+"_subjects") {
			return false, nil
		}
		if !qdrantCollectionExists(t, namespace+"_objects") {
			return false, nil
		}
		if !qdrantCollectionExists(t, namespace+"_subjects_dense") {
			return false, nil
		}
		if !qdrantCollectionExists(t, namespace+"_objects_dense") {
			return false, nil
		}
		if qdrantPointCount(t, namespace+"_subjects") == 0 {
			return false, nil
		}
		if qdrantPointCount(t, namespace+"_objects") == 0 {
			return false, nil
		}
		if qdrantPointCount(t, namespace+"_subjects_dense") == 0 {
			return false, nil
		}
		if qdrantPointCount(t, namespace+"_objects_dense") == 0 {
			return false, nil
		}
		card, ttl := trendingKeyState(t, namespace)
		if card == 0 {
			return false, nil
		}
		if ttl <= 0 {
			return false, nil
		}
		return true, nil
	})

	resp := doRequest(t, http.MethodGet, baseURL+"/v1/namespaces/"+namespace+"/trending", apiKey, nil)
	var body struct {
		Namespace string `json:"namespace"`
		Items     []struct {
			ObjectID string  `json:"object_id"`
			Score    float64 `json:"score"`
		} `json:"items"`
	}
	decodeJSON(t, resp, &body)

	if body.Namespace != namespace {
		t.Fatalf("namespace = %q, want %q", body.Namespace, namespace)
	}
	if len(body.Items) == 0 {
		t.Fatal("expected non-empty trending items after cron recompute")
	}
}

func TestCronRecompute_NamespaceWithoutEventsProducesNoArtifacts(t *testing.T) {
	activeNamespace, _ := createIsolatedNamespace(t, "cron_active", map[string]any{
		"action_weights":  map[string]float64{"VIEW": 1.0, "LIKE": 4.0},
		"lambda":          0.01,
		"gamma":           0.5,
		"max_results":     20,
		"dense_strategy":  "svd",
		"embedding_dim":   2,
		"alpha":           0.7,
		"dense_distance":  "cosine",
		"trending_window": 24,
		"trending_ttl":    120,
		"lambda_trending": 0.1,
		"seen_items_days": 30,
	})
	idleNamespace, _ := createIsolatedNamespace(t, "cron_idle", map[string]any{
		"action_weights":  map[string]float64{"VIEW": 1.0, "LIKE": 4.0},
		"lambda":          0.01,
		"gamma":           0.5,
		"max_results":     20,
		"dense_strategy":  "svd",
		"embedding_dim":   2,
		"alpha":           0.7,
		"dense_distance":  "cosine",
		"trending_window": 24,
		"trending_ttl":    120,
		"lambda_trending": 0.1,
		"seen_items_days": 30,
	})

	now := time.Now().UTC().Truncate(time.Second)
	seedEvent(t, activeNamespace, "user_a", "item_1", "VIEW", 1.0, now.Add(-15*time.Minute), nil)
	seedEvent(t, activeNamespace, "user_b", "item_2", "LIKE", 4.0, now.Add(-10*time.Minute), nil)

	runCronOnceUntil(t, 20*time.Second, func() (bool, error) {
		if !qdrantCollectionExists(t, activeNamespace+"_subjects") {
			return false, nil
		}
		if qdrantPointCount(t, activeNamespace+"_subjects") == 0 {
			return false, nil
		}
		card, _ := trendingKeyState(t, activeNamespace)
		return card > 0, nil
	})

	for _, collection := range []string{
		idleNamespace + "_subjects",
		idleNamespace + "_objects",
		idleNamespace + "_subjects_dense",
		idleNamespace + "_objects_dense",
	} {
		if qdrantCollectionExists(t, collection) {
			t.Fatalf("unexpected Qdrant collection for idle namespace: %s", collection)
		}
	}

	card, ttl := trendingKeyState(t, idleNamespace)
	if card != 0 {
		t.Fatalf("idle namespace trending card = %d, want 0", card)
	}
	if ttl >= 0 {
		t.Fatalf("idle namespace trending ttl = %v, want missing key sentinel", ttl)
	}
}
