//go:build e2e

package e2e

import (
	"context"
	"testing"
	"time"
)

const ingestStreamName = "codohue:events"

func TestIngestStream_ValidEventPersistsToPostgres(t *testing.T) {
	namespace, _ := createIsolatedNamespace(t, "ingest_stream", map[string]any{
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
	objectCreatedAt := occurredAt.Add(-24 * time.Hour)

	publishEvent(t, map[string]any{
		"namespace":         namespace,
		"subject_id":        "stream_user_1",
		"object_id":         "stream_item_1",
		"action":            "LIKE",
		"timestamp":         occurredAt.Format(time.RFC3339),
		"object_created_at": objectCreatedAt.Format(time.RFC3339),
	})

	waitForEventPersisted(t, namespace, "stream_user_1", "stream_item_1")

	var got struct {
		Action          string
		Weight          float64
		OccurredAt      time.Time
		ObjectCreatedAt *time.Time
	}
	err := testDB.QueryRow(context.Background(), `
		SELECT action, weight, occurred_at, object_created_at
		FROM events
		WHERE namespace = $1 AND subject_id = $2 AND object_id = $3
	`, namespace, "stream_user_1", "stream_item_1").Scan(
		&got.Action,
		&got.Weight,
		&got.OccurredAt,
		&got.ObjectCreatedAt,
	)
	if err != nil {
		t.Fatalf("query persisted event: %v", err)
	}

	if got.Action != "LIKE" {
		t.Fatalf("action = %q, want LIKE", got.Action)
	}
	if got.Weight != 3.0 {
		t.Fatalf("weight = %.1f, want 3.0", got.Weight)
	}
	if !got.OccurredAt.Truncate(time.Second).Equal(occurredAt) {
		t.Fatalf("occurred_at = %v, want %v", got.OccurredAt, occurredAt)
	}
	if got.ObjectCreatedAt == nil || !got.ObjectCreatedAt.Truncate(time.Second).Equal(objectCreatedAt) {
		t.Fatalf("object_created_at = %v, want %v", got.ObjectCreatedAt, objectCreatedAt)
	}
}

func TestIngestStream_DefaultWeightUsedWhenActionNotConfigured(t *testing.T) {
	namespace, _ := createIsolatedNamespace(t, "ingest_default_weight", map[string]any{
		"action_weights": map[string]float64{"LIKE": 2.0},
		"lambda":         0.01,
		"gamma":          0.5,
		"max_results":    20,
		"dense_strategy": "byoe",
		"embedding_dim":  4,
		"alpha":          0.7,
		"dense_distance": "cosine",
	})

	publishEvent(t, map[string]any{
		"namespace":  namespace,
		"subject_id": "stream_user_2",
		"object_id":  "stream_item_2",
		"action":     "VIEW",
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	})

	waitForEventPersisted(t, namespace, "stream_user_2", "stream_item_2")

	var weight float64
	err := testDB.QueryRow(context.Background(), `
		SELECT weight
		FROM events
		WHERE namespace = $1 AND subject_id = $2 AND object_id = $3
	`, namespace, "stream_user_2", "stream_item_2").Scan(&weight)
	if err != nil {
		t.Fatalf("query persisted weight: %v", err)
	}
	if weight != 1.0 {
		t.Fatalf("weight = %.1f, want default VIEW weight 1.0", weight)
	}
}

func TestIngestStream_InvalidJSONDoesNotPersistRow(t *testing.T) {
	namespace, _ := createIsolatedNamespace(t, "ingest_invalid_json", nil)

	publishRawEvent(t, `{"namespace":`)

	time.Sleep(500 * time.Millisecond)

	var count int
	err := testDB.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM events
		WHERE namespace = $1
	`, namespace).Scan(&count)
	if err != nil {
		t.Fatalf("count events: %v", err)
	}
	if count != 0 {
		t.Fatalf("event count = %d, want 0", count)
	}
}

func TestIngestStream_MissingRequiredFieldDoesNotPersistRow(t *testing.T) {
	namespace, _ := createIsolatedNamespace(t, "ingest_missing_field", nil)

	publishEvent(t, map[string]any{
		"namespace":  namespace,
		"subject_id": "stream_user_3",
		"action":     "LIKE",
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	})

	time.Sleep(500 * time.Millisecond)

	var count int
	err := testDB.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM events
		WHERE namespace = $1 AND subject_id = $2
	`, namespace, "stream_user_3").Scan(&count)
	if err != nil {
		t.Fatalf("count invalid events: %v", err)
	}
	if count != 0 {
		t.Fatalf("event count = %d, want 0", count)
	}
}
