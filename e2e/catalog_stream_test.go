//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

const catalogStreamName = "codohue:catalog"

func publishCatalogStreamItem(t testing.TB, payload map[string]any) string {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal catalog payload: %v", err)
	}
	id, err := testRedis.XAdd(context.Background(), &goredis.XAddArgs{
		Stream: catalogStreamName,
		Values: map[string]any{"payload": string(data)},
	}).Result()
	if err != nil {
		t.Fatalf("publish catalog item: %v", err)
	}
	return id
}

// TestCatalogStream_DurableIngest is the US4 outage property: the producer
// fire-and-forgets an XADD with no API interaction, and the ingest worker
// persists the row whenever it gets to it. The XADD itself needs only Redis —
// content produced during a full Codohue outage sits in the stream untrimmed
// until consumed, which is the same consume path this test exercises.
func TestCatalogStream_DurableIngest(t *testing.T) {
	namespace, _ := createIsolatedNamespace(t, "catalog_stream", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0},
		"dense_source":   "byoe",
		"embedding_dim":  128,
	})
	enableCatalogForNamespace(t, namespace, 128)

	publishCatalogStreamItem(t, map[string]any{
		"namespace":         namespace,
		"object_id":         "post-stream-1",
		"content":           "durable transport catalog content",
		"author_subject_id": "author-1",
	})

	// The worker consumes, persists catalog_items, and publishes to the
	// embed stream — same lifecycle as HTTP ingest from here on.
	waitForRowCount(t, 15*time.Second, `
		SELECT COUNT(*) FROM catalog_items
		WHERE namespace = $1 AND object_id = $2`, 1, namespace, "post-stream-1")

	// Attribution flowed through to the objects table.
	waitForRowCount(t, 10*time.Second, `
		SELECT COUNT(*) FROM objects
		WHERE namespace = $1 AND object_id = $2 AND author_subject_id = $3`,
		1, namespace, "post-stream-1", "author-1")

	// Redelivery of identical content is an idempotent no-op: still one row.
	publishCatalogStreamItem(t, map[string]any{
		"namespace": namespace,
		"object_id": "post-stream-1",
		"content":   "durable transport catalog content",
	})
	waitForRowCount(t, 10*time.Second, `
		SELECT COUNT(*) FROM catalog_items
		WHERE namespace = $1 AND object_id = $2`, 1, namespace, "post-stream-1")
}

// TestCatalogBatchIngest_AndReconciliation covers the repair-pass pair: batch
// ingest with per-item results, then the reconciliation read listing what the
// namespace holds.
func TestCatalogBatchIngest_AndReconciliation(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "catalog_batch", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0},
		"dense_source":   "byoe",
		"embedding_dim":  128,
	})
	enableCatalogForNamespace(t, namespace, 128)

	resp := doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+namespace+"/catalog/batch", apiKey, map[string]any{
		"items": []map[string]any{
			{"object_id": "post-b1", "content": "batch item one"},
			{"object_id": "post-b2", "content": "batch item two"},
			{"object_id": "post-b3", "content": "   "}, // rejected per-item
		},
	})
	assertStatus(t, resp, http.StatusAccepted)
	var batch struct {
		Accepted int `json:"accepted"`
		Rejected int `json:"rejected"`
		Results  []struct {
			ObjectID string `json:"object_id"`
			Accepted bool   `json:"accepted"`
			Error    string `json:"error"`
		} `json:"results"`
	}
	decodeJSON(t, resp, &batch)
	if batch.Accepted != 2 || batch.Rejected != 1 {
		t.Fatalf("batch counts: %+v", batch)
	}
	if batch.Results[2].Error != "empty_content" {
		t.Fatalf("per-item code: %+v", batch.Results[2])
	}

	resp = doRequest(t, http.MethodGet, baseURL+"/v1/namespaces/"+namespace+"/catalog/objects?limit=10", apiKey, nil)
	assertStatus(t, resp, http.StatusOK)
	var objects struct {
		Total int `json:"total"`
		Items []struct {
			ObjectID  string `json:"object_id"`
			UpdatedAt string `json:"updated_at"`
		} `json:"items"`
	}
	decodeJSON(t, resp, &objects)
	if objects.Total != 2 || len(objects.Items) != 2 {
		t.Fatalf("reconciliation read: %+v", objects)
	}
	for _, it := range objects.Items {
		if it.UpdatedAt == "" {
			t.Fatalf("missing updated_at: %+v", it)
		}
	}
}
