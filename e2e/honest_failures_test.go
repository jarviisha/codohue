//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jarviisha/codohue/internal/core/nslifecycle"
)

// A client cannot act on a failure it cannot classify. These are the three
// answers the data plane owes it: 404 means the namespace will never accept
// this write, 409 means not yet, 503 means we could not tell. Before this the
// whole set collapsed into 500, so a client retried writes that could never
// land and gave up on ones that would.
func TestHonestFailures_MissingNamespaceIsStable404AcrossEveryWritePath(t *testing.T) {
	ghost := newTestNamespace(t, "ghost")

	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{
			name: "event ingest", method: http.MethodPost,
			path: "/v1/namespaces/" + ghost + "/events",
			body: map[string]any{"subject_id": "u1", "object_id": "o1", "action": "VIEW"},
		},
		{
			name: "catalog ingest", method: http.MethodPost,
			path: "/v1/namespaces/" + ghost + "/catalog",
			body: map[string]any{"object_id": "o1", "content": "hello"},
		},
		{
			name: "object metadata", method: http.MethodPut,
			path: "/v1/namespaces/" + ghost + "/objects/o1",
			body: map[string]any{"author_subject_id": "u1"},
		},
		{
			name: "byoe object embedding", method: http.MethodPut,
			path: "/v1/namespaces/" + ghost + "/objects/o1/embedding",
			body: map[string]any{"vector": []float32{0.1, 0.2}},
		},
		{
			name: "recommendations", method: http.MethodGet,
			path: "/v1/namespaces/" + ghost + "/subjects/u1/recommendations",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The global admin key is deliberately used here: an
			// authorization tier does not make a namespace exist, so even the
			// most privileged credential gets the same answer.
			resp := doRequest(t, tc.method, baseURL+tc.path, adminKey, tc.body)
			defer resp.Body.Close() //nolint:errcheck

			if resp.StatusCode != http.StatusNotFound {
				t.Fatalf("got %d, want 404 for a namespace that does not exist", resp.StatusCode)
			}
		})
	}

	// Nothing may have been created by the rejected writes.
	for _, table := range []string{"events", "catalog_items", "objects", "id_mappings", "namespace_configs"} {
		assertNoNamespaceRows(t, table, ghost)
	}
	assertQdrantNamespaceAbsent(t, ghost)
}

// A rejected read must not leave a cached answer behind: the recommendation
// cache has a 5-minute TTL, so one default-backed response would outlive the
// outage that produced it.
func TestHonestFailures_RejectedReadsAreNeverCached(t *testing.T) {
	ghost := newTestNamespace(t, "ghost_cache")

	for i := 0; i < 3; i++ {
		resp := doRequest(t, http.MethodGet,
			baseURL+"/v1/namespaces/"+ghost+"/subjects/u1/recommendations", adminKey, nil)
		status := resp.StatusCode
		resp.Body.Close() //nolint:errcheck
		if status != http.StatusNotFound {
			t.Fatalf("attempt %d: got %d, want a stable 404", i, status)
		}
	}

	// The cache key prefix for a namespace is owned by nslifecycle, so the
	// check asks it rather than re-deriving the shape here.
	prefix := nslifecycle.MustPhysicalName(nslifecycle.KindRecommendationCache, ghost, 1)
	keys, err := testRedis.Keys(context.Background(), prefix+":*").Result()
	if err != nil {
		t.Fatalf("scan recommendation cache: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("a rejected read left cache entries: %v", keys)
	}
}

// Catalog ingest commits content and attribution together. A request naming an
// author must either land both or neither — reporting 202 with the attribution
// silently dropped left the caller no way to learn it never happened.
func TestHonestFailures_CatalogAttributionIsAtomicWithContent(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "atomic_attr", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0},
		"embedding_dim":  128,
		"dense_source":   "byoe",
	})
	enableCatalogForNamespace(t, namespace, 128)
	objectID := "atomic-obj-1"

	resp := doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+namespace+"/catalog", apiKey,
		map[string]any{"object_id": objectID, "content": "hello world", "author_subject_id": "author-1"})
	assertStatus(t, resp, http.StatusAccepted)
	resp.Body.Close() //nolint:errcheck

	waitForRowCount(t, 10*time.Second,
		`SELECT COUNT(*) FROM catalog_items WHERE namespace = $1 AND object_id = $2`, 1, namespace, objectID)
	waitForRowCount(t, 10*time.Second,
		`SELECT COUNT(*) FROM objects WHERE namespace = $1 AND object_id = $2 AND author_subject_id = $3`,
		1, namespace, objectID, "author-1")

	// Re-ingesting identical content with a new author updates attribution
	// without redoing embed work — otherwise correcting an author would mean
	// editing the content to force a write.
	resp = doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+namespace+"/catalog", apiKey,
		map[string]any{"object_id": objectID, "content": "hello world", "author_subject_id": "author-2"})
	assertStatus(t, resp, http.StatusAccepted)
	resp.Body.Close() //nolint:errcheck

	waitForRowCount(t, 10*time.Second,
		`SELECT COUNT(*) FROM objects WHERE namespace = $1 AND object_id = $2 AND author_subject_id = $3`,
		1, namespace, objectID, "author-2")
}

// Deleting an object touches two Qdrant collections and a PostgreSQL row.
// Deleting one that was never ingested is a no-op rather than a failure, and a
// repeated delete stays idempotent — a caller retrying after a partial failure
// must be able to reach success.
func TestHonestFailures_ObjectDeleteIsIdempotent(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "idem_delete", defaultNamespaceConfig())

	for i := 0; i < 3; i++ {
		resp := doRequest(t, http.MethodDelete,
			baseURL+"/v1/namespaces/"+namespace+"/objects/never-ingested", apiKey, nil)
		status := resp.StatusCode
		resp.Body.Close() //nolint:errcheck
		if status != http.StatusNoContent {
			t.Fatalf("attempt %d: got %d, want 204", i, status)
		}
	}
}

// A future creation timestamp gets its own error code so a client can tell it
// apart from a malformed vector — both are 400s, but they need different fixes.
func TestHonestFailures_FutureObjectCreatedAtHasItsOwnCode(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "future_ts", defaultNamespaceConfig())
	future := "2999-01-01T00:00:00Z"

	resp := doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+namespace+"/events", apiKey,
		map[string]any{"subject_id": "u1", "object_id": "o1", "action": "VIEW", "object_created_at": future})
	code, _ := decodeErrorJSON(t, resp, http.StatusBadRequest)
	if code != "invalid_object_created_at" {
		t.Errorf("event ingest error code = %q, want invalid_object_created_at", code)
	}

	resp = doRequest(t, http.MethodPut, baseURL+"/v1/namespaces/"+namespace+"/objects/o1/embedding", apiKey,
		map[string]any{"vector": []float32{0.1, 0.2}, "object_created_at": future})
	code, _ = decodeErrorJSON(t, resp, http.StatusBadRequest)
	if code != "invalid_object_created_at" {
		t.Errorf("byoe error code = %q, want invalid_object_created_at", code)
	}

	assertNoNamespaceRows(t, "events", namespace)
}
