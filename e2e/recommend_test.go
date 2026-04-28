//go:build e2e

package e2e

import (
	"net/http"
	"testing"
	"time"
)

func TestRecommend_ColdStart(t *testing.T) {
	// A brand-new subject with no interaction history. The service falls back
	// to trending → popular. Either way the HTTP response must be 200.
	resp := doRequest(t, http.MethodGet,
		baseURL+"/v1/recommendations?subject_id=cold_user&namespace="+testNS,
		nsKey, nil)

	var body struct {
		SubjectID   string    `json:"subject_id"`
		Namespace   string    `json:"namespace"`
		Items       []struct {
			ObjectID string `json:"object_id"`
		} `json:"items"`
		Source      string    `json:"source"`
		GeneratedAt time.Time `json:"generated_at"`
	}
	decodeJSON(t, resp, &body)

	if body.SubjectID != "cold_user" {
		t.Errorf("subject_id = %q, want %q", body.SubjectID, "cold_user")
	}
	if body.Namespace != testNS {
		t.Errorf("namespace = %q, want %q", body.Namespace, testNS)
	}
	if body.Source == "" {
		t.Error("source must not be empty")
	}
	if body.GeneratedAt.IsZero() {
		t.Error("generated_at is zero")
	}
	if body.Items == nil {
		t.Error("items must not be null (expected empty array for cold start with no popular items)")
	}
}

func TestRecommend_Unauthorized(t *testing.T) {
	resp := doRequest(t, http.MethodGet,
		baseURL+"/v1/recommendations?subject_id=u1&namespace="+testNS,
		"bad-key", nil)
	assertStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()
}

func TestRecommend_NoToken(t *testing.T) {
	resp := doRequest(t, http.MethodGet,
		baseURL+"/v1/recommendations?subject_id=u1&namespace="+testNS,
		"", nil)
	assertStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()
}

func TestRecommend_MissingSubjectID(t *testing.T) {
	resp := doRequest(t, http.MethodGet,
		baseURL+"/v1/recommendations?namespace="+testNS,
		nsKey, nil)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestRecommend_MissingNamespace(t *testing.T) {
	resp := doRequest(t, http.MethodGet,
		baseURL+"/v1/recommendations?subject_id=u1",
		nsKey, nil)
	// Namespace is missing → auth middleware skips validation and passes through →
	// handler returns 400 "subject_id and namespace are required".
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestRecommend_InvalidLimit(t *testing.T) {
	resp := doRequest(t, http.MethodGet,
		baseURL+"/v1/recommendations?subject_id=u1&namespace="+testNS+"&limit=abc",
		nsKey, nil)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestRecommend_ZeroLimit(t *testing.T) {
	resp := doRequest(t, http.MethodGet,
		baseURL+"/v1/recommendations?subject_id=u1&namespace="+testNS+"&limit=0",
		nsKey, nil)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestRecommend_LimitCapped(t *testing.T) {
	// Namespace config sets max_results=20; result items must not exceed that.
	resp := doRequest(t, http.MethodGet,
		baseURL+"/v1/recommendations?subject_id=cold_user&namespace="+testNS+"&limit=5",
		nsKey, nil)

	var body struct {
		Items []struct {
			ObjectID string `json:"object_id"`
		} `json:"items"`
	}
	decodeJSON(t, resp, &body)

	if len(body.Items) > 5 {
		t.Errorf("items count = %d, want ≤ 5", len(body.Items))
	}
}
