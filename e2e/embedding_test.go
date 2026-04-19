//go:build e2e

package e2e

import (
	"net/http"
	"testing"
)

// dim4 is a valid vector that matches the testNS config (embedding_dim=4).
var dim4 = map[string]any{"vector": []float32{0.1, 0.2, 0.3, 0.4}}

// --- Object embeddings ---

func TestStoreObjectEmbedding_Success(t *testing.T) {
	url := baseURL + "/v1/objects/" + testNS + "/obj_embed_1/embedding"
	resp := doRequest(t, http.MethodPost, url, nsKey, dim4)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
}

func TestStoreObjectEmbedding_Idempotent(t *testing.T) {
	// Storing the same vector twice must succeed (upsert semantics).
	url := baseURL + "/v1/objects/" + testNS + "/obj_embed_1/embedding"
	resp := doRequest(t, http.MethodPost, url, nsKey, dim4)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
}

func TestStoreObjectEmbedding_Unauthorized(t *testing.T) {
	url := baseURL + "/v1/objects/" + testNS + "/obj_embed_2/embedding"
	resp := doRequest(t, http.MethodPost, url, "bad-key", dim4)
	assertStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()
}

func TestStoreObjectEmbedding_NoToken(t *testing.T) {
	url := baseURL + "/v1/objects/" + testNS + "/obj_embed_2/embedding"
	resp := doRequest(t, http.MethodPost, url, "", dim4)
	assertStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()
}

func TestStoreObjectEmbedding_EmptyVector(t *testing.T) {
	url := baseURL + "/v1/objects/" + testNS + "/obj_embed_3/embedding"
	resp := doRequest(t, http.MethodPost, url, nsKey, map[string]any{"vector": []float32{}})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestStoreObjectEmbedding_MissingVector(t *testing.T) {
	url := baseURL + "/v1/objects/" + testNS + "/obj_embed_3/embedding"
	resp := doRawPost(t, url, nsKey, `{}`)
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

func TestStoreObjectEmbedding_DimMismatch(t *testing.T) {
	// testNS has embedding_dim=4; a 3-element vector must be rejected before hitting Qdrant.
	url := baseURL + "/v1/objects/" + testNS + "/obj_embed_4/embedding"
	resp := doRequest(t, http.MethodPost, url, nsKey,
		map[string]any{"vector": []float32{0.1, 0.2, 0.3}})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

// --- Subject embeddings ---

func TestStoreSubjectEmbedding_Success(t *testing.T) {
	url := baseURL + "/v1/subjects/" + testNS + "/subj_embed_1/embedding"
	resp := doRequest(t, http.MethodPost, url, nsKey, dim4)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
}

func TestStoreSubjectEmbedding_Unauthorized(t *testing.T) {
	url := baseURL + "/v1/subjects/" + testNS + "/subj_embed_2/embedding"
	resp := doRequest(t, http.MethodPost, url, "bad-key", dim4)
	assertStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()
}

func TestStoreSubjectEmbedding_DimMismatch(t *testing.T) {
	url := baseURL + "/v1/subjects/" + testNS + "/subj_embed_3/embedding"
	resp := doRequest(t, http.MethodPost, url, nsKey,
		map[string]any{"vector": []float32{0.5, 0.6}})
	assertStatus(t, resp, http.StatusBadRequest)
	resp.Body.Close()
}

// --- Delete object ---
// These tests run after embedding tests so the dense collection already exists.

func TestDeleteObject_Success(t *testing.T) {
	// Store an embedding first to ensure the object has a Qdrant point.
	storeURL := baseURL + "/v1/objects/" + testNS + "/obj_to_delete/embedding"
	store := doRequest(t, http.MethodPost, storeURL, nsKey, dim4)
	assertStatus(t, store, http.StatusNoContent)
	store.Body.Close()

	deleteURL := baseURL + "/v1/objects/" + testNS + "/obj_to_delete"
	resp := doRequest(t, http.MethodDelete, deleteURL, nsKey, nil)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
}

func TestDeleteObject_Idempotent(t *testing.T) {
	// Deleting a non-existent object must also return 204.
	url := baseURL + "/v1/objects/" + testNS + "/obj_never_existed"
	resp := doRequest(t, http.MethodDelete, url, nsKey, nil)
	assertStatus(t, resp, http.StatusNoContent)
	resp.Body.Close()
}

func TestDeleteObject_Unauthorized(t *testing.T) {
	url := baseURL + "/v1/objects/" + testNS + "/obj_embed_1"
	resp := doRequest(t, http.MethodDelete, url, "bad-key", nil)
	assertStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()
}

func TestDeleteObject_NoToken(t *testing.T) {
	url := baseURL + "/v1/objects/" + testNS + "/obj_embed_1"
	resp := doRequest(t, http.MethodDelete, url, "", nil)
	assertStatus(t, resp, http.StatusUnauthorized)
	resp.Body.Close()
}
