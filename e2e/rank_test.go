//go:build e2e

package e2e

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestRank_ColdStart(t *testing.T) {
	// Subject with no interaction history — service returns candidates in original
	// order with score 0 (rankFallback path).
	body := map[string]any{
		"subject_id": "rank_cold_user",
		"namespace":  testNS,
		"candidates": []string{"item_a", "item_b", "item_c"},
	}
	resp := doRequest(t, http.MethodPost, baseURL+"/v1/rank", nsKey, body)

	var result struct {
		SubjectID string `json:"subject_id"`
		Namespace string `json:"namespace"`
		Items     []struct {
			ObjectID string  `json:"object_id"`
			Score    float64 `json:"score"`
		} `json:"items"`
		Source      string    `json:"source"`
		GeneratedAt time.Time `json:"generated_at"`
	}
	decodeJSON(t, resp, &result)

	if result.SubjectID != "rank_cold_user" {
		t.Errorf("subject_id = %q, want %q", result.SubjectID, "rank_cold_user")
	}
	if result.Namespace != testNS {
		t.Errorf("namespace = %q, want %q", result.Namespace, testNS)
	}
	if len(result.Items) != 3 {
		t.Errorf("items count = %d, want 3", len(result.Items))
	}
	if result.GeneratedAt.IsZero() {
		t.Error("generated_at is zero")
	}
}

func TestRank_EmptyCandidates(t *testing.T) {
	body := map[string]any{
		"subject_id": "rank_cold_user",
		"namespace":  testNS,
		"candidates": []string{},
	}
	resp := doRequest(t, http.MethodPost, baseURL+"/v1/rank", nsKey, body)

	var result struct {
		Items []any `json:"items"`
	}
	decodeJSON(t, resp, &result)

	if len(result.Items) != 0 {
		t.Errorf("items count = %d, want 0 for empty candidates", len(result.Items))
	}
}

func TestRank_Unauthorized(t *testing.T) {
	body := map[string]any{
		"subject_id": "u1",
		"namespace":  testNS,
		"candidates": []string{"item_a"},
	}
	resp := doRequest(t, http.MethodPost, baseURL+"/v1/rank", "bad-key", body)
	assertStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()
}

func TestRank_MissingSubjectID(t *testing.T) {
	body := map[string]any{
		"namespace":  testNS,
		"candidates": []string{"item_a"},
	}
	resp := doRequest(t, http.MethodPost, baseURL+"/v1/rank", nsKey, body)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestRank_MissingNamespace(t *testing.T) {
	body := map[string]any{
		"subject_id": "u1",
		"candidates": []string{"item_a"},
	}
	resp := doRequest(t, http.MethodPost, baseURL+"/v1/rank", nsKey, body)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestRank_InvalidBody(t *testing.T) {
	resp := doRawPost(t, baseURL+"/v1/rank", nsKey, "not-json-at-all")
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestRank_TooManyCandidates(t *testing.T) {
	candidates := make([]string, 501)
	for i := range candidates {
		candidates[i] = fmt.Sprintf("item_%d", i)
	}
	body := map[string]any{
		"subject_id": "u1",
		"namespace":  testNS,
		"candidates": candidates,
	}
	resp := doRequest(t, http.MethodPost, baseURL+"/v1/rank", nsKey, body)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}
