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
		"dense_source":    "disabled",
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
			Scored   bool   `json:"scored"`
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
		if !item.Scored {
			t.Errorf("item %q: scored = false on the CF path, want true", item.ObjectID)
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
		"dense_source":    "disabled",
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
			Scored   bool   `json:"scored"`
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
	// The fallback ranks the namespace, not this subject: the 0 it reports is
	// a placeholder and must be labelled as one.
	for _, item := range body.Items {
		if item.Scored {
			t.Errorf("item %q: scored = true on the fallback path, want false", item.ObjectID)
		}
	}
}

// TestRecommendComputed_EscapedSubjectIDServesTheSameSubject exercises the
// spelling a real Bluesky client sends. A DID contains reserved characters, so
// clients escape it; chi matches on the raw path, and before the route
// boundary decoded its parameters the escaped spelling was an unknown subject
// and silently fell back to popular items.
func TestRecommendComputed_EscapedSubjectIDServesTheSameSubject(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "recommend_escaped_id", map[string]any{
		"action_weights":  map[string]float64{"VIEW": 1.0, "LIKE": 4.0},
		"lambda":          0.01,
		"gamma":           0.5,
		"max_results":     10,
		"seen_items_days": 30,
		"dense_source":    "disabled",
	})

	// Escaped by hand: url.PathEscape leaves ':' alone, and the encoded
	// spelling reaching the router is the whole point of the test.
	const subjectID = "did:plc:e2eescapedsubject"
	const escaped = "did%3Aplc%3Ae2eescapedsubject"

	// The subject needs enough interactions to clear the cold-start threshold,
	// or both spellings land on a fallback and the comparison proves nothing.
	now := time.Now().UTC().Truncate(time.Second)
	seedEvent(t, namespace, subjectID, "item_1", "LIKE", 4.0, now.Add(-50*time.Minute), nil)
	seedEvent(t, namespace, subjectID, "item_2", "LIKE", 4.0, now.Add(-48*time.Minute), nil)
	seedEvent(t, namespace, subjectID, "item_1", "VIEW", 1.0, now.Add(-46*time.Minute), nil)
	seedEvent(t, namespace, subjectID, "item_2", "VIEW", 1.0, now.Add(-44*time.Minute), nil)
	seedEvent(t, namespace, subjectID, "item_1", "LIKE", 4.0, now.Add(-42*time.Minute), nil)
	seedEvent(t, namespace, subjectID, "item_2", "LIKE", 4.0, now.Add(-40*time.Minute), nil)
	seedEvent(t, namespace, "peer_user", "item_2", "LIKE", 4.0, now.Add(-38*time.Minute), nil)
	seedEvent(t, namespace, "peer_user", "item_3", "LIKE", 4.0, now.Add(-35*time.Minute), nil)
	seedEvent(t, namespace, "peer_user", "item_4", "VIEW", 1.0, now.Add(-30*time.Minute), nil)

	runCronOnceUntil(t, 20*time.Second, func() (bool, error) {
		return qdrantCollectionExists(t, namespace+"_subjects") &&
			qdrantPointCount(t, namespace+"_subjects") > 0, nil
	})

	type recBody struct {
		SubjectID string `json:"subject_id"`
		Items     []struct {
			ObjectID string `json:"object_id"`
			Scored   bool   `json:"scored"`
		} `json:"items"`
		Source string `json:"source"`
	}

	get := func(pathID string) recBody {
		t.Helper()
		resp := doRequest(t, http.MethodGet,
			baseURL+"/v1/namespaces/"+namespace+"/subjects/"+pathID+"/recommendations?limit=3",
			apiKey, nil)
		var body recBody
		decodeJSON(t, resp, &body)
		return body
	}

	literal := get(subjectID)
	encoded := get(escaped)

	if literal.Source != "collaborative_filtering" {
		t.Fatalf("literal source = %q, want collaborative_filtering (the fixture did not warm up)", literal.Source)
	}
	if encoded.SubjectID != subjectID {
		t.Errorf("encoded subject_id = %q, want the decoded %q", encoded.SubjectID, subjectID)
	}
	if encoded.Source != literal.Source {
		t.Errorf("encoded source = %q, want %q — the two spellings must name one subject",
			encoded.Source, literal.Source)
	}
	if len(encoded.Items) != len(literal.Items) {
		t.Fatalf("encoded returned %d items, literal returned %d", len(encoded.Items), len(literal.Items))
	}
	for i := range literal.Items {
		if encoded.Items[i].ObjectID != literal.Items[i].ObjectID {
			t.Errorf("items[%d]: encoded = %q, literal = %q", i, encoded.Items[i].ObjectID, literal.Items[i].ObjectID)
		}
		if !encoded.Items[i].Scored {
			t.Errorf("items[%d]: encoded spelling came back unscored", i)
		}
	}
}
