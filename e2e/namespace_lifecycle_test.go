//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/jarviisha/codohue/internal/core/nslifecycle"
	goredis "github.com/redis/go-redis/v9"
)

// lifecycleRaceIterations is the trial count from the story's independent
// test. One or two rounds would pass by luck; the failure this guards against
// is a writer that slipped between the delete's last cleanup step and the
// tombstone landing, which only shows up under repetition.
const lifecycleRaceIterations = 100

// A delete that reports success must be authoritative. Before the lifecycle
// ledger, a writer in flight during the wipe could re-create rows for a
// namespace the operator had just been told was gone — and nothing ever
// cleaned them, because no namespace_configs row pointed at them.
func TestNamespaceLifecycle_ConcurrentWritersNeverOutliveADelete(t *testing.T) {
	ensureAdminServer(t)

	for iteration := 0; iteration < lifecycleRaceIterations; iteration++ {
		namespace, apiKey := createIsolatedNamespace(t,
			fmt.Sprintf("race_%d", iteration), defaultNamespaceConfig())

		var writers sync.WaitGroup
		stop := make(chan struct{})
		for writer := 0; writer < 4; writer++ {
			writers.Add(1)
			go func(id int) {
				defer writers.Done()
				for seq := 0; ; seq++ {
					select {
					case <-stop:
						return
					default:
					}
					// Every one of these is expected to be accepted OR
					// rejected — never accepted-then-orphaned.
					resp := doRequest(t, http.MethodPost,
						baseURL+"/v1/namespaces/"+namespace+"/events", apiKey,
						map[string]any{
							"subject_id": fmt.Sprintf("u%d", id),
							"object_id":  fmt.Sprintf("o%d_%d", id, seq),
							"action":     "VIEW",
						})
					resp.Body.Close() //nolint:errcheck
				}
			}(writer)
		}

		// Let the writers get going, then delete underneath them.
		time.Sleep(20 * time.Millisecond)
		resp := doRequest(t, http.MethodDelete,
			adminBaseURL+"/api/admin/v1/namespaces/"+namespace, adminKey, nil)
		deleteStatus := resp.StatusCode
		resp.Body.Close() //nolint:errcheck

		close(stop)
		writers.Wait()

		if deleteStatus != http.StatusOK {
			t.Fatalf("iteration %d: delete returned %d", iteration, deleteStatus)
		}

		// The delete said the namespace is gone. Nothing may remain.
		for _, table := range []string{"events", "catalog_items", "objects", "id_mappings", "namespace_configs"} {
			assertNoNamespaceRows(t, table, namespace)
		}
		assertQdrantNamespaceAbsent(t, namespace)
	}
}

// Recreating a name mints a new generation, and the new incarnation's physical
// artifacts are separately named. A delayed writer holding the old generation
// therefore cannot make old data visible to the new namespace's readers.
func TestNamespaceLifecycle_RecreateMintsANewGeneration(t *testing.T) {
	ensureAdminServer(t)

	namespace, apiKey := createIsolatedNamespace(t, "recreate", defaultNamespaceConfig())

	first := namespaceLifecycleGeneration(t, namespace)
	if first != 1 {
		t.Fatalf("first generation = %d, want 1", first)
	}

	resp := doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+namespace+"/events", apiKey,
		map[string]any{"subject_id": "u1", "object_id": "o1", "action": "VIEW"})
	assertStatus(t, resp, http.StatusAccepted)
	resp.Body.Close() //nolint:errcheck
	waitForEventPersisted(t, namespace, "u1", "o1")

	resp = doRequest(t, http.MethodDelete, adminBaseURL+"/api/admin/v1/namespaces/"+namespace, adminKey, nil)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close() //nolint:errcheck

	newKey := createNamespace(t, namespace, defaultNamespaceConfig())
	second := namespaceLifecycleGeneration(t, namespace)
	if second != first+1 {
		t.Fatalf("recreated generation = %d, want %d", second, first+1)
	}

	// The recreated namespace starts empty: the previous incarnation's events
	// were deleted, and nothing the old generation writes can reach it.
	assertNoNamespaceRows(t, "events", namespace)

	resp = doRequest(t, http.MethodPost, baseURL+"/v1/namespaces/"+namespace+"/events", newKey,
		map[string]any{"subject_id": "u1", "object_id": "o2", "action": "VIEW"})
	assertStatus(t, resp, http.StatusAccepted)
	resp.Body.Close() //nolint:errcheck
	waitForEventPersisted(t, namespace, "u1", "o2")

	// Physical names are qualified for generation 2+, so the two incarnations
	// cannot share a trending key or an embed stream.
	legacyTrending := nslifecycle.MustPhysicalName(nslifecycle.KindTrending, namespace, 1)
	currentTrending := nslifecycle.MustPhysicalName(nslifecycle.KindTrending, namespace, second)
	if legacyTrending == currentTrending {
		t.Fatalf("generation %d reuses generation 1's trending key %q", second, currentTrending)
	}
}

// A stream entry stamped with a superseded generation is stale work: it is
// ACKed and dropped rather than retried, because no retry can make it valid.
// Leaving it pending would block the PEL forever.
func TestNamespaceLifecycle_StaleGenerationEnvelopesAreDroppedNotRetried(t *testing.T) {
	namespace, _ := createIsolatedNamespace(t, "stale_gen", defaultNamespaceConfig())
	generation := namespaceLifecycleGeneration(t, namespace)

	stale := generation + 99
	id := publishEvent(t, map[string]any{
		"namespace":            namespace,
		"namespace_generation": stale,
		"subject_id":           "u1",
		"object_id":            "o-stale",
		"action":               "VIEW",
	})

	// The entry must leave the PEL (ACKed) without producing a row.
	waitForCondition(t, 15*time.Second, func() (bool, error) {
		progress := redisGroupProgressFor(t, "codohue:events", "codohue-ingest")
		return progress.Pending == 0, nil
	})
	assertNoNamespaceRows(t, "events", namespace)
	t.Logf("stale entry %s was acked and dropped", id)
}

// A legacy (generation-less) envelope is accepted only while the global gate
// is open and only for generation 1 — the grandfathering window that lets
// existing producers keep working during the rollout.
func TestNamespaceLifecycle_LegacyEnvelopesAreAcceptedOnlyForGenerationOne(t *testing.T) {
	ensureAdminServer(t)

	namespace, _ := createIsolatedNamespace(t, "legacy_env", defaultNamespaceConfig())
	if got := namespaceLifecycleGeneration(t, namespace); got != 1 {
		t.Fatalf("generation = %d, want a fresh generation-1 namespace", got)
	}

	publishEvent(t, map[string]any{
		"namespace":  namespace,
		"subject_id": "u1",
		"object_id":  "o-legacy",
		"action":     "VIEW",
	})
	waitForEventPersisted(t, namespace, "u1", "o-legacy")

	// After a recreate the namespace is generation 2, so the same
	// generation-less envelope is stale.
	resp := doRequest(t, http.MethodDelete, adminBaseURL+"/api/admin/v1/namespaces/"+namespace, adminKey, nil)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close() //nolint:errcheck
	createNamespace(t, namespace, defaultNamespaceConfig())

	publishEvent(t, map[string]any{
		"namespace":  namespace,
		"subject_id": "u1",
		"object_id":  "o-legacy-after-recreate",
		"action":     "VIEW",
	})
	waitForCondition(t, 15*time.Second, func() (bool, error) {
		progress := redisGroupProgressFor(t, "codohue:events", "codohue-ingest")
		return progress.Pending == 0, nil
	})
	assertNoNamespaceRows(t, "events", namespace)
}

// Deleting a namespace removes its Redis artifacts for the generation it held,
// including the embed stream — a stream left behind would keep an embedder
// consumer alive against a namespace that no longer exists.
func TestNamespaceLifecycle_DeleteRemovesGenerationScopedRedisKeys(t *testing.T) {
	ensureAdminServer(t)

	namespace, _ := createIsolatedNamespace(t, "redis_wipe", defaultNamespaceConfig())
	generation := namespaceLifecycleGeneration(t, namespace)

	embedStream := nslifecycle.MustPhysicalName(nslifecycle.KindEmbedStream, namespace, generation)
	trendingKey := nslifecycle.MustPhysicalName(nslifecycle.KindTrending, namespace, generation)
	ctx := context.Background()
	if err := testRedis.XAdd(ctx, &goredis.XAddArgs{
		Stream: embedStream, Values: map[string]any{"catalog_item_id": 1},
	}).Err(); err != nil {
		t.Fatalf("seed embed stream: %v", err)
	}
	if err := testRedis.ZAdd(ctx, trendingKey, goredis.Z{Score: 1, Member: "o1"}).Err(); err != nil {
		t.Fatalf("seed trending: %v", err)
	}

	resp := doRequest(t, http.MethodDelete, adminBaseURL+"/api/admin/v1/namespaces/"+namespace, adminKey, nil)
	assertStatus(t, resp, http.StatusOK)
	resp.Body.Close() //nolint:errcheck

	remaining, err := testRedis.Exists(ctx, embedStream, trendingKey).Result()
	if err != nil {
		t.Fatalf("check redis keys: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("%d generation-%d redis key(s) survived the delete", remaining, generation)
	}
}
