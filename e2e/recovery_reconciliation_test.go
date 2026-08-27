//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	infraredis "github.com/jarviisha/codohue/internal/infra/redis"
	"github.com/jarviisha/codohue/pkg/codohuetypes"
	goredis "github.com/redis/go-redis/v9"
)

// A reclaim scan that restarts at "0-0" every tick re-examines the head of the
// PEL forever, so a permanently failing entry at the front starves every entry
// behind it. This walks three pages worth of pending entries with a failing
// head and asserts the later ones are still reached.
func TestRecovery_ReclaimVisitsEveryPageDespiteAFailingHead(t *testing.T) {
	ctx := context.Background()
	stream := "e2e:reclaim:" + strconv.FormatInt(time.Now().UnixNano(), 10)
	group := "codohue-ingest"
	t.Cleanup(func() { testRedis.Del(ctx, stream) }) //nolint:errcheck
	ensureRedisGroup(t, stream, group, "0")

	const entries = 25
	for i := 0; i < entries; i++ {
		if err := testRedis.XAdd(ctx, &goredis.XAddArgs{
			Stream: stream,
			Values: map[string]any{"seq": i},
		}).Err(); err != nil {
			t.Fatalf("xadd: %v", err)
		}
	}

	// Deliver everything to a consumer that never acks, so the whole set sits
	// in the PEL — the state a crashed replica leaves behind.
	if _, err := testRedis.XReadGroup(ctx, &goredis.XReadGroupArgs{
		Group: group, Consumer: "dead-replica", Streams: []string{stream, ">"}, Count: entries,
	}).Result(); err != nil {
		t.Fatalf("xreadgroup: %v", err)
	}

	progress := redisGroupProgressFor(t, stream, group)
	if progress.Pending != entries {
		t.Fatalf("pending = %d, want %d", progress.Pending, entries)
	}

	// Reclaim in small pages, carrying the cursor exactly as the workers do.
	cursor := "0-0"
	seen := map[string]bool{}
	for page := 0; page < 10; page++ {
		messages, next, err := testRedis.XAutoClaim(ctx, &goredis.XAutoClaimArgs{
			Stream: stream, Group: group, Consumer: "live-replica",
			MinIdle: 0, Start: cursor, Count: 10,
		}).Result()
		if err != nil {
			t.Fatalf("xautoclaim page %d: %v", page, err)
		}
		for _, message := range messages {
			seen[message.ID] = true
		}
		cursor = next
		if next == "0-0" || next == "" {
			break
		}
	}

	if len(seen) != entries {
		t.Fatalf("reclaim reached %d of %d entries; a cursor that reset would stall at the head", len(seen), entries)
	}
}

// Retention respects the reclaim frontier: an entry that is pending in any
// group is below the safe frontier and must survive the trim, or a reclaim
// would find its work deleted.
func TestRecovery_RetentionNeverTrimsPendingWork(t *testing.T) {
	ctx := context.Background()
	stream := "e2e:reclaim_retention:" + strconv.FormatInt(time.Now().UnixNano(), 10)
	group := "codohue-ingest"
	t.Cleanup(func() { testRedis.Del(ctx, stream) }) //nolint:errcheck
	ensureRedisGroup(t, stream, group, "0")

	for i := 0; i < 10; i++ {
		if err := testRedis.XAdd(ctx, &goredis.XAddArgs{Stream: stream, Values: map[string]any{"seq": i}}).Err(); err != nil {
			t.Fatalf("xadd: %v", err)
		}
	}
	// Read all ten, ack only the first five: entries 6-10 stay pending.
	messages, err := testRedis.XReadGroup(ctx, &goredis.XReadGroupArgs{
		Group: group, Consumer: "c1", Streams: []string{stream, ">"}, Count: 10,
	}).Result()
	if err != nil {
		t.Fatalf("xreadgroup: %v", err)
	}
	var ids []string
	for _, s := range messages {
		for _, message := range s.Messages {
			ids = append(ids, message.ID)
		}
	}
	if err := testRedis.XAck(ctx, stream, group, ids[:5]...).Err(); err != nil {
		t.Fatalf("xack: %v", err)
	}

	retention := infraredis.NewRetention(testRedis, false)
	if _, err := retention.RunOnce(ctx, infraredis.StreamSpec{
		Name: stream, Kind: "events", ExpectedGroups: []string{group},
	}); err != nil {
		t.Fatalf("retention pass: %v", err)
	}

	length, err := testRedis.XLen(ctx, stream).Result()
	if err != nil {
		t.Fatalf("xlen: %v", err)
	}
	if length < 5 {
		t.Fatalf("stream length %d: retention trimmed entries that are still pending", length)
	}
}

// Catalog reconciliation pages by (updated_at, id). A batch ingest gives many
// rows the same updated_at, so offset paging over a set that is still being
// written re-sends rows or skips them; the keyset cursor is stable because id
// breaks the tie.
func TestRecovery_CatalogReconciliationPagesThroughEqualTimestamps(t *testing.T) {
	namespace, apiKey := createIsolatedNamespace(t, "reconcile", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0},
		"embedding_dim":  128,
		"dense_source":   "byoe",
	})
	enableCatalogForNamespace(t, namespace, 128)

	// One batch: every row lands with the same NOW().
	const total = 12
	items := make([]map[string]any, 0, total)
	for i := 0; i < total; i++ {
		items = append(items, map[string]any{
			"object_id": fmt.Sprintf("obj-%02d", i),
			"content":   fmt.Sprintf("content number %d", i),
		})
	}
	resp := doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+namespace+"/catalog/batch", apiKey,
		map[string]any{"items": items})
	assertStatus(t, resp, http.StatusAccepted)
	resp.Body.Close() //nolint:errcheck

	waitForRowCount(t, 20*time.Second,
		`SELECT COUNT(*) FROM catalog_items WHERE namespace = $1`, total, namespace)

	// Force the tie: rewrite every updated_at to one instant.
	if _, err := testDB.Exec(context.Background(), `
		UPDATE catalog_items SET updated_at = TIMESTAMPTZ '2026-08-25 10:00:00Z'
		WHERE namespace = $1`, namespace); err != nil {
		t.Fatalf("flatten updated_at: %v", err)
	}

	seen := map[string]int{}
	cursor := ""
	pages := 0
	for {
		url := fmt.Sprintf("%s/v1/namespaces/%s/catalog/objects?limit=5", baseURL, namespace)
		if cursor != "" {
			url += "&cursor=" + cursor
		}
		page := doRequest(t, http.MethodGet, url, apiKey, nil)
		var body codohuetypes.CatalogObjectsResponse
		decodeJSON(t, page, &body)
		pages++

		for _, item := range body.Items {
			seen[item.ObjectID]++
		}
		if body.NextCursor == "" {
			break
		}
		cursor = body.NextCursor
		if pages > 10 {
			t.Fatal("reconciliation did not terminate")
		}
	}

	if pages < 3 {
		t.Fatalf("walked %d page(s); the test needs at least three to be meaningful", pages)
	}
	if len(seen) != total {
		t.Fatalf("visited %d distinct objects, want %d — the keyset skipped rows", len(seen), total)
	}
	for objectID, count := range seen {
		if count != 1 {
			t.Errorf("object %q returned %d times; equal timestamps caused a duplicate", objectID, count)
		}
	}
}

// A cursor is only meaningful for the query that produced it. Replaying one
// against a different namespace would silently page through another result
// set, so it is rejected rather than reinterpreted.
func TestRecovery_CatalogCursorIsBoundToItsQuery(t *testing.T) {
	first, firstKey := createIsolatedNamespace(t, "cursor_a", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0},
		"embedding_dim":  128,
		"dense_source":   "byoe",
	})
	enableCatalogForNamespace(t, first, 128)
	second, secondKey := createIsolatedNamespace(t, "cursor_b", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0},
		"embedding_dim":  128,
		"dense_source":   "byoe",
	})
	enableCatalogForNamespace(t, second, 128)

	for i := 0; i < 3; i++ {
		resp := doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+first+"/catalog", firstKey,
			map[string]any{"object_id": fmt.Sprintf("a-%d", i), "content": "hello"})
		assertStatus(t, resp, http.StatusAccepted)
		resp.Body.Close() //nolint:errcheck
	}
	waitForRowCount(t, 20*time.Second, `SELECT COUNT(*) FROM catalog_items WHERE namespace = $1`, 3, first)

	page := doRequest(t, http.MethodGet,
		fmt.Sprintf("%s/v1/namespaces/%s/catalog/objects?limit=1", baseURL, first), firstKey, nil)
	var body codohuetypes.CatalogObjectsResponse
	decodeJSON(t, page, &body)
	if body.NextCursor == "" {
		t.Fatal("expected a next_cursor on a full page")
	}

	replayed := doRequest(t, http.MethodGet,
		fmt.Sprintf("%s/v1/namespaces/%s/catalog/objects?limit=1&cursor=%s", baseURL, second, body.NextCursor),
		secondKey, nil)
	defer replayed.Body.Close() //nolint:errcheck
	if replayed.StatusCode != http.StatusBadRequest {
		t.Fatalf("cross-namespace cursor replay returned %d, want 400", replayed.StatusCode)
	}

	malformed := doRequest(t, http.MethodGet,
		fmt.Sprintf("%s/v1/namespaces/%s/catalog/objects?cursor=!!!not-base64!!!", baseURL, first), firstKey, nil)
	defer malformed.Body.Close() //nolint:errcheck
	if malformed.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed cursor returned %d, want 400 rather than a silent restart", malformed.StatusCode)
	}
}
