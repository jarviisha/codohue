//go:build e2e

package e2e

import (
	"net/http"
	"strings"
	"testing"
)

func TestClientErrors_UnauthorizedIsJSON(t *testing.T) {
	resp := doRequest(t, http.MethodGet,
		baseURL+"/v1/namespaces/"+testNS+"/recommendations?subject_id=u1",
		"bad-key", nil)

	code, message := decodeErrorJSON(t, resp, http.StatusUnauthorized)
	if code != "unauthorized" {
		t.Fatalf("error code = %q, want unauthorized", code)
	}
	if !strings.Contains(message, "bearer token") {
		t.Fatalf("error message = %q, want bearer token hint", message)
	}
}

func TestClientErrors_InvalidRequestIsJSON(t *testing.T) {
	resp := doRawPost(t, baseURL+"/v1/namespaces/"+testNS+"/rank", nsKey, "not-json-at-all")

	code, message := decodeErrorJSON(t, resp, http.StatusBadRequest)
	if code != "invalid_request" {
		t.Fatalf("error code = %q, want invalid_request", code)
	}
	if message != "invalid request body" {
		t.Fatalf("error message = %q", message)
	}
}

func TestClientErrors_NamespaceMismatchIsJSON(t *testing.T) {
	resp := doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+testNS+"/rank", nsKey, map[string]any{
		"namespace":  testNS + "_other",
		"subject_id": "u1",
		"candidates": []string{"item_a"},
	})

	code, message := decodeErrorJSON(t, resp, http.StatusBadRequest)
	if code != "namespace_mismatch" {
		t.Fatalf("error code = %q, want namespace_mismatch", code)
	}
	if !strings.Contains(message, "path and body") {
		t.Fatalf("error message = %q", message)
	}
}

func TestClientErrors_EmbeddingDimMismatchIsJSON(t *testing.T) {
	resp := doRequest(t, http.MethodPost,
		baseURL+"/v1/namespaces/"+testNS+"/objects/obj_bad_dim/embedding",
		nsKey, map[string]any{"vector": []float32{0.1, 0.2, 0.3}})

	code, message := decodeErrorJSON(t, resp, http.StatusBadRequest)
	if code != "embedding_dimension_mismatch" {
		t.Fatalf("error code = %q, want embedding_dimension_mismatch", code)
	}
	if !strings.Contains(message, "embedding dimension mismatch") {
		t.Fatalf("error message = %q", message)
	}
}

func TestClientErrors_HTTPIngestUnknownActionIsJSON(t *testing.T) {
	resp := doRequest(t, http.MethodPost,
		baseURL+"/v1/namespaces/"+testNS+"/events",
		nsKey, map[string]any{
			"subject_id": "u1",
			"object_id":  "o1",
			"action":     "UNKNOWN",
		})

	code, message := decodeErrorJSON(t, resp, http.StatusBadRequest)
	if code != "invalid_event" {
		t.Fatalf("error code = %q, want invalid_event", code)
	}
	if !strings.Contains(message, "unknown action") {
		t.Fatalf("error message = %q", message)
	}
}
