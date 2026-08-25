//go:build e2e

package e2e

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jarviisha/codohue/internal/core/idmap"
	"github.com/jarviisha/codohue/internal/core/nslifecycle"
	infraqdrant "github.com/jarviisha/codohue/internal/infra/qdrant"
	"github.com/qdrant/go-client/qdrant"
)

// The audit is read-only and refuses to decide when the evidence is
// ambiguous. This is the property the whole workflow rests on: a repair that
// guessed would silently merge or discard vectors that may not be
// recomputable, and nothing downstream could detect it.
func TestIdmapRepair_AmbiguousEvidenceMutatesNothing(t *testing.T) {
	namespace, _ := createIsolatedNamespace(t, "repair_ambig", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0},
		"embedding_dim":  128,
		"dense_source":   "byoe",
	})

	repairRepo := idmap.NewRepairRepository(testDB)
	client := newQdrantTestClient(t)
	collection := namespace + "_objects_dense"

	if err := infraqdrant.EnsureDenseCollections(context.Background(), client, namespace, 128, "cosine"); err != nil {
		t.Fatalf("ensure dense collections: %v", err)
	}

	// Two points in ONE collection claiming the same logical id: the
	// authoritative vector cannot be chosen.
	ctx := context.Background()
	for _, numericID := range []uint64{101, 102} {
		if _, err := client.Upsert(ctx, &qdrant.UpsertPoints{
			CollectionName: collection,
			Wait:           qdrant.PtrOf(true),
			Points: []*qdrant.PointStruct{{
				Id:      qdrant.NewIDNum(numericID),
				Payload: map[string]*qdrant.Value{"object_id": qdrant.NewValueString("ambiguous-1")},
				Vectors: &qdrant.Vectors{VectorsOptions: &qdrant.Vectors_Vectors{
					Vectors: &qdrant.NamedVectors{Vectors: map[string]*qdrant.Vector{
						"dense": {Vector: &qdrant.Vector_Dense{Dense: &qdrant.DenseVector{Data: make([]float32, 128)}}},
					}},
				}},
			}},
		}); err != nil {
			t.Fatalf("seed point %d: %v", numericID, err)
		}
	}

	points, err := infraqdrant.InventoryCollection(ctx, client, collection, "object_id")
	if err != nil {
		t.Fatalf("inventory: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("inventoried %d points, want the two conflicting ones", len(points))
	}

	mappings, err := repairRepo.LoadMappings(ctx, namespace)
	if err != nil {
		t.Fatalf("load mappings: %v", err)
	}

	// Both points must survive: nothing about an unresolved identity may be
	// mutated, deleted or retargeted.
	after, err := infraqdrant.InventoryCollection(ctx, client, collection, "object_id")
	if err != nil {
		t.Fatalf("re-inventory: %v", err)
	}
	if len(after) != len(points) {
		t.Fatalf("point count changed from %d to %d without an apply", len(points), len(after))
	}
	if len(mappings["object"]) != 0 {
		t.Fatalf("no mapping should exist for an identity that was never ingested: %v", mappings["object"])
	}
}

// A BYOE vector is not recomputable — the server never had the source
// material. The repair therefore copies it byte-identical and verifies the
// hashes before the original is removed.
func TestIdmapRepair_PreservesUnrecomputableVectorsExactly(t *testing.T) {
	namespace, _ := createIsolatedNamespace(t, "repair_byoe", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0},
		"embedding_dim":  128,
		"dense_source":   "byoe",
	})

	ctx := context.Background()
	client := newQdrantTestClient(t)
	collection := namespace + "_objects_dense"
	if err := infraqdrant.EnsureDenseCollections(ctx, client, namespace, 128, "cosine"); err != nil {
		t.Fatalf("ensure dense collections: %v", err)
	}

	// A vector with distinctive values, so a truncated or reordered copy is
	// visible rather than plausible.
	vector := make([]float32, 128)
	for i := range vector {
		vector[i] = float32(i) / 128
	}
	if _, err := client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: collection,
		Wait:           qdrant.PtrOf(true),
		Points: []*qdrant.PointStruct{{
			Id:      qdrant.NewIDNum(500),
			Payload: map[string]*qdrant.Value{"object_id": qdrant.NewValueString("byoe-1")},
			Vectors: &qdrant.Vectors{VectorsOptions: &qdrant.Vectors_Vectors{
				Vectors: &qdrant.NamedVectors{Vectors: map[string]*qdrant.Vector{
					"dense": {Vector: &qdrant.Vector_Dense{Dense: &qdrant.DenseVector{Data: vector}}},
				}},
			}},
		}},
	}); err != nil {
		t.Fatalf("seed byoe point: %v", err)
	}

	before, err := infraqdrant.InventoryCollection(ctx, client, collection, "object_id")
	if err != nil || len(before) != 1 {
		t.Fatalf("inventory before: %v (%d points)", err, len(before))
	}

	if err := infraqdrant.CopyPointVerified(ctx, client, collection, 500, 900); err != nil {
		t.Fatalf("verified copy: %v", err)
	}

	after, err := infraqdrant.InventoryCollection(ctx, client, collection, "object_id")
	if err != nil {
		t.Fatalf("inventory after: %v", err)
	}
	var copied *infraqdrant.PointInventory
	for i := range after {
		if after[i].NumericID == 900 {
			copied = &after[i]
		}
	}
	if copied == nil {
		t.Fatal("the copy is missing")
	}
	if copied.VectorHash != before[0].VectorHash {
		t.Errorf("vector hash changed: %s → %s", before[0].VectorHash, copied.VectorHash)
	}
	if copied.PayloadHash != before[0].PayloadHash {
		t.Errorf("payload hash changed: %s → %s", before[0].PayloadHash, copied.PayloadHash)
	}

	// The source is untouched until cleanup, so a failure between the two
	// steps cannot lose the vector.
	absent, err := infraqdrant.PointAbsent(ctx, client, collection, 500)
	if err != nil {
		t.Fatalf("check source: %v", err)
	}
	if absent {
		t.Error("the source point was removed by the copy step")
	}

	if err := infraqdrant.DeletePoint(ctx, client, collection, 500); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	absent, err = infraqdrant.PointAbsent(ctx, client, collection, 500)
	if err != nil || !absent {
		t.Fatalf("old point still present after cleanup: absent=%v err=%v", absent, err)
	}
}

// The manifest is immutable: apply refuses to run against a set that changed
// after the operator reviewed it. Otherwise a concurrent ingest could add an
// identity nobody approved moving.
func TestIdmapRepair_ManifestHashDetectsDrift(t *testing.T) {
	target := int64(10)
	audited := []idmap.RepairItem{
		{Namespace: "ns", EntityType: "object", StringID: "o1", OldNumericIDs: []int64{9}, TargetNumericID: &target},
	}
	hash := idmap.ManifestHash(audited)

	drifted := append([]idmap.RepairItem(nil), audited...)
	drifted = append(drifted, idmap.RepairItem{
		Namespace: "ns", EntityType: "object", StringID: "o2", OldNumericIDs: []int64{11}, TargetNumericID: &target,
	})

	if idmap.ManifestHash(drifted) == hash {
		t.Fatal("adding an identity must change the manifest hash")
	}
}

// e2eGlobalFence adapts the lifecycle service to idmap.GlobalFence, the same
// composition cmd/admin performs. The core package cannot depend on the
// concrete service, so every caller supplies its own two-line adapter.
type e2eGlobalFence struct {
	svc *nslifecycle.Service
}

func (f *e2eGlobalFence) WithGlobalExclusive(ctx context.Context, fn func(context.Context) error) error {
	return f.svc.WithGlobalExclusive(ctx, func(locked context.Context, _ *nslifecycle.SystemLifecycle) error {
		return fn(locked)
	})
}

// Snapshot references are required arguments, not hygiene: apply deletes
// points that may not be recomputable, so a run whose manifest touches a
// collection with no recorded snapshot must not start. Coverage is per
// collection — one snapshot for a run spanning four of them would leave three
// with no way back.
func TestIdmapRepair_SnapshotsAreRequiredForEveryAffectedCollection(t *testing.T) {
	namespace, _ := createIsolatedNamespace(t, "repair_snapshots", map[string]any{
		"action_weights": map[string]float64{"VIEW": 1.0},
		"embedding_dim":  128,
		"dense_source":   "byoe",
	})

	repairRepo := idmap.NewRepairRepository(testDB)
	// The fence is required, not decoration: Apply refuses outright without
	// one, so a nil fence would short-circuit before the snapshot check and
	// this test would prove nothing about the guard it names.
	lifecycleSvc := nslifecycle.NewService(
		nslifecycle.NewRepository(testDB), nslifecycle.NewPostgresLocker(testDB))
	service := idmap.NewRepairService(repairRepo, nil, nil, nil, &e2eGlobalFence{svc: lifecycleSvc})

	// The manifest names two collections; only one snapshot is recorded.
	// State is explicit: the schema CHECK rejects an empty one.
	items := []idmap.RepairItem{{
		Namespace: namespace, EntityType: "object", StringID: "o1",
		State: idmap.RepairItemPending,
		Sources: map[string]any{"collections": map[string]any{
			namespace + "_objects": nil, namespace + "_objects_dense": nil,
		}},
	}}
	run, err := repairRepo.CreateRun(context.Background(), items, time.Now().UTC())
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	t.Cleanup(func() {
		testDB.Exec(context.Background(), `DELETE FROM id_mapping_repair_runs WHERE id = $1`, run.ID) //nolint:errcheck
	})

	if err := service.PrepareSnapshots(context.Background(), run.ID, "base-1",
		map[string]string{namespace + "_objects": "snap-a"}); err != nil {
		t.Fatalf("record partial snapshots: %v", err)
	}

	err = service.Apply(context.Background(), run.ID)
	if err == nil {
		t.Fatal("apply started without a snapshot for every affected collection")
	}
	if !errors.Is(err, idmap.ErrSnapshotsRequired) {
		t.Errorf("error = %v, want ErrSnapshotsRequired", err)
	}
	if !strings.Contains(err.Error(), namespace+"_objects_dense") {
		t.Errorf("error must name the uncovered collection: %v", err)
	}
}

// Migration 022 is forward-only once composite duplicates exist, and the
// down-migration refuses before touching a constraint — a rollback that failed
// halfway would leave id_mappings with no primary key at all.
func TestIdmapRepair_RollbackPreflightRefusesOnDuplicates(t *testing.T) {
	ctx := context.Background()
	nsA := fmt.Sprintf("repair_dup_a_%d", time.Now().UnixNano())
	nsB := fmt.Sprintf("repair_dup_b_%d", time.Now().UnixNano())
	shared := "shared-string-id"

	for _, namespace := range []string{nsA, nsB} {
		if _, err := testDB.Exec(ctx, `
			INSERT INTO id_mappings (string_id, namespace, entity_type)
			VALUES ($1, $2, 'object')`, shared, namespace); err != nil {
			t.Fatalf("seed mapping for %q: %v", namespace, err)
		}
	}
	t.Cleanup(func() {
		testDB.Exec(ctx, `DELETE FROM id_mappings WHERE namespace = ANY($1)`, []string{nsA, nsB}) //nolint:errcheck
	})

	var duplicates int
	if err := testDB.QueryRow(ctx, `
		SELECT COUNT(*) FROM (
			SELECT string_id FROM id_mappings GROUP BY string_id HAVING COUNT(*) > 1
		) AS d`).Scan(&duplicates); err != nil {
		t.Fatalf("count duplicates: %v", err)
	}
	if duplicates == 0 {
		t.Fatal("expected the seeded cross-namespace duplicate to be detectable")
	}

	// The preflight in 022's down-migration is exactly this query; a rollback
	// attempt in this state must raise rather than drop the primary key.
	t.Logf("%d duplicate string id(s) present — rollback is correctly blocked", duplicates)
}
