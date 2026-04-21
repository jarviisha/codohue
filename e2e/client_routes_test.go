//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestClientRoutes_HTTPIngestPersistsEvent(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "client_http_ingest", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0, "LIKE": 3.0},
		"lambda":         0.01,
		"gamma":          0.5,
		"max_results":    20,
		"dense_strategy": "byoe",
		"embedding_dim":  4,
		"alpha":          0.7,
		"dense_distance": "cosine",
	})

	occurredAt := time.Now().UTC().Truncate(time.Second)
	resp := doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+namespace+"/events", apiKey, map[string]any{
		"subject_id": "http_user_1",
		"object_id":  "http_item_1",
		"action":     "LIKE",
		"timestamp":  occurredAt.Format(time.RFC3339),
	})
	assertStatus(t, resp, http.StatusAccepted)
	resp.Body.Close()

	waitForEventPersisted(t, namespace, "http_user_1", "http_item_1")

	var weight float64
	err := testDB.QueryRow(context.Background(), `
		SELECT weight
		FROM events
		WHERE namespace = $1 AND subject_id = $2 AND object_id = $3
	`, namespace, "http_user_1", "http_item_1").Scan(&weight)
	if err != nil {
		t.Fatalf("query persisted event: %v", err)
	}
	if weight != 3.0 {
		t.Fatalf("weight = %.1f, want 3.0", weight)
	}
}

func TestClientRoutes_RecommendationsByNamespace(t *testing.T) {
	resp := doRequest(t, http.MethodGet,
		baseURL+"/v1/namespaces/"+testNS+"/recommendations?subject_id=cold_user",
		nsKey, nil)

	var body struct {
		SubjectID string   `json:"subject_id"`
		Namespace string   `json:"namespace"`
		Items     []string `json:"items"`
	}
	decodeJSON(t, resp, &body)

	if body.SubjectID != "cold_user" {
		t.Fatalf("subject_id = %q, want cold_user", body.SubjectID)
	}
	if body.Namespace != testNS {
		t.Fatalf("namespace = %q, want %q", body.Namespace, testNS)
	}
	if body.Items == nil {
		t.Fatal("items must not be null")
	}
}

func TestClientRoutes_RankByNamespace(t *testing.T) {
	resp := doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+testNS+"/rank", nsKey, map[string]any{
		"subject_id": "rank_cold_user",
		"candidates": []string{"item_a", "item_b"},
	})

	var body struct {
		SubjectID string `json:"subject_id"`
		Namespace string `json:"namespace"`
		Items     []struct {
			ObjectID string `json:"object_id"`
		} `json:"items"`
	}
	decodeJSON(t, resp, &body)

	if body.SubjectID != "rank_cold_user" {
		t.Fatalf("subject_id = %q, want rank_cold_user", body.SubjectID)
	}
	if body.Namespace != testNS {
		t.Fatalf("namespace = %q, want %q", body.Namespace, testNS)
	}
	if len(body.Items) != 2 {
		t.Fatalf("items count = %d, want 2", len(body.Items))
	}
}

func TestClientRoutes_TrendingByNamespace(t *testing.T) {
	resp := doRequest(t, http.MethodGet, baseURL+"/v1/namespaces/"+testNS+"/trending?window_hours=48", nsKey, nil)

	var body struct {
		Namespace   string `json:"namespace"`
		WindowHours int    `json:"window_hours"`
	}
	decodeJSON(t, resp, &body)

	if body.Namespace != testNS {
		t.Fatalf("namespace = %q, want %q", body.Namespace, testNS)
	}
	if body.WindowHours != 48 {
		t.Fatalf("window_hours = %d, want 48", body.WindowHours)
	}
}

func TestClientRoutes_ObjectEmbeddingByNamespace(t *testing.T) {
	url := baseURL + "/v1/namespaces/" + testNS + "/objects/client_obj_1/embedding"
	resp := doRequest(t, http.MethodPost, url, nsKey, dim4)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
}

func TestClientRoutes_SubjectEmbeddingByNamespace(t *testing.T) {
	url := baseURL + "/v1/namespaces/" + testNS + "/subjects/client_subject_1/embedding"
	resp := doRequest(t, http.MethodPost, url, nsKey, dim4)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
}

func TestClientRoutes_DeleteObjectByNamespace(t *testing.T) {
	storeURL := baseURL + "/v1/namespaces/" + testNS + "/objects/client_obj_to_delete/embedding"
	store := doRequest(t, http.MethodPost, storeURL, nsKey, dim4)
	assertStatus(t, store, http.StatusNoContent)
	store.Body.Close()

	deleteURL := baseURL + "/v1/namespaces/" + testNS + "/objects/client_obj_to_delete"
	resp := doRequest(t, http.MethodDelete, deleteURL, nsKey, nil)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
}
