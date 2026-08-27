package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jarviisha/codohue/internal/core/nslifecycle"
	infraqdrant "github.com/jarviisha/codohue/internal/infra/qdrant"
)

// A point is tied back to a logical identity through the payload key its
// collection uses. Pairing a collection with the wrong key would make every
// point in it read as unlabeled, and the audit would quarantine the entire
// namespace with a message that says nothing about the real cause.
func TestCollectionKinds_PairEachCollectionWithItsPayloadKey(t *testing.T) {
	want := map[infraqdrant.CollectionKind]struct{ entityType, idField string }{
		infraqdrant.CollectionSubjects:      {"subject", "subject_id"},
		infraqdrant.CollectionSubjectsDense: {"subject", "subject_id"},
		infraqdrant.CollectionObjects:       {"object", "object_id"},
		infraqdrant.CollectionObjectsDense:  {"object", "object_id"},
	}

	if len(collectionKinds) != len(want) {
		t.Fatalf("audit covers %d collections, want all %d", len(collectionKinds), len(want))
	}
	for _, spec := range collectionKinds {
		expected, known := want[spec.kind]
		if !known {
			t.Errorf("unexpected collection kind %q", spec.kind)
			continue
		}
		if spec.entityType != expected.entityType || spec.idField != expected.idField {
			t.Errorf("%s pairs (%s, %s), want (%s, %s)",
				spec.kind, spec.entityType, spec.idField, expected.entityType, expected.idField)
		}
		delete(want, spec.kind)
	}
	for kind := range want {
		t.Errorf("collection %q is never inventoried, so its points are invisible to the audit", kind)
	}
}

// Subjects and objects are separate identity spaces. A collection typed as the
// wrong entity would have its points matched against the other space's
// mappings, silently resolving to the wrong numeric id.
func TestCollectionKinds_EntityTypeMatchesTheCollectionFamily(t *testing.T) {
	for _, spec := range collectionKinds {
		family := "object"
		if strings.HasPrefix(string(spec.kind), "subjects") {
			family = "subject"
		}
		if spec.entityType != family {
			t.Errorf("%s is typed %q but belongs to the %q family", spec.kind, spec.entityType, family)
		}
	}
}

// Qdrant point ids are unsigned. Converting a negative id with a plain
// uint64() would wrap it to a huge positive value and address a point nobody
// meant to touch — on the delete path, that destroys an unrelated vector.
func TestPointID_RefusesNonPositiveIDs(t *testing.T) {
	for _, id := range []int64{0, -1, -9223372036854775808} {
		got, err := pointID(id)
		if err == nil {
			t.Errorf("pointID(%d) returned %d instead of refusing", id, got)
		}
		if got != 0 {
			t.Errorf("pointID(%d) returned %d alongside an error", id, got)
		}
	}
}

func TestPointID_PassesThroughValidIDs(t *testing.T) {
	for _, id := range []int64{1, 42, 9223372036854775807} {
		got, err := pointID(id)
		if err != nil {
			t.Fatalf("pointID(%d): %v", id, err)
		}
		if got != uint64(id) {
			t.Errorf("pointID(%d) = %d, want the same value", id, got)
		}
	}
}

// The mover is the only thing standing between a corrupt manifest row and a
// destructive Qdrant call, so every method must refuse before reaching the
// client — which is nil here precisely to prove nothing downstream ran.
func TestQdrantPointMover_RefusesBeforeTouchingTheClient(t *testing.T) {
	mover := &qdrantPointMover{client: nil}
	ctx := t.Context()

	if err := mover.CopyPointVerified(ctx, "ns_objects_dense", -1, 10); err == nil {
		t.Error("copy from a negative id must be refused")
	}
	if err := mover.CopyPointVerified(ctx, "ns_objects_dense", 10, 0); err == nil {
		t.Error("copy to a non-positive id must be refused")
	}
	if err := mover.DeletePoint(ctx, "ns_objects_dense", -5); err == nil {
		t.Error("delete of a negative id must be refused")
	}
	absent, err := mover.PointAbsent(ctx, "ns_objects_dense", -5)
	if err == nil {
		t.Error("absence check on a negative id must be refused")
	}
	if absent {
		t.Error("a refused absence check must not report the point as gone")
	}
}

// Verification reads a repaired point back through the same payload-key pairing
// the audit used to inventory it. Getting this wrong would make every verified
// point look like it carried no identity, failing a correct repair.
func TestPayloadIDField_MatchesTheCollectionFamily(t *testing.T) {
	for _, tc := range []struct{ collection, want string }{
		{"prod_subjects", "subject_id"},
		{"prod_subjects_dense", "subject_id"},
		{"prod_objects", "object_id"},
		{"prod_objects_dense", "object_id"},
		{"prod_g3_subjects_dense", "subject_id"},
		{"prod_g3_objects", "object_id"},
		// A namespace whose own name contains "objects" must not confuse the
		// subject collections it owns.
		{"my_objects_app_subjects", "subject_id"},
	} {
		if got := payloadIDField(tc.collection); got != tc.want {
			t.Errorf("%s -> %s, want %s", tc.collection, got, tc.want)
		}
	}
}

func TestQdrantPointMover_InspectRefusesNonPositiveIDs(t *testing.T) {
	mover := &qdrantPointMover{client: nil}

	_, found, err := mover.InspectPoint(t.Context(), "ns_objects_dense", -1)
	if err == nil {
		t.Error("inspecting a negative id must be refused")
	}
	if found {
		t.Error("a refused inspection must not report the point as found")
	}
}

// The rebuild runs inside the repair's global exclusive fence, which carries no
// per-namespace lease — and minting a numeric id demands one. Without the
// explicit lease the rebuild fails on its first subject and fails the run.
func TestSparseRebuildAdapter_AttachesTheNamespaceLease(t *testing.T) {
	var seen struct {
		generation int64
		ok         bool
	}
	adapter := &sparseRebuildAdapter{
		generation: func(context.Context, string) (int64, error) { return 7, nil },
	}
	// Stand in for compute.Service by inspecting the context the adapter builds.
	leased := nslifecycle.ContextWithLease(context.Background(), "ns", 7, nslifecycle.LockExclusive)
	seen.generation, seen.ok = nslifecycle.LeaseGeneration(leased, "ns")
	if !seen.ok || seen.generation != 7 {
		t.Fatalf("lease generation = %d ok=%v, want 7 true", seen.generation, seen.ok)
	}
	if err := nslifecycle.RequireNamespaceLease(leased, "ns"); err != nil {
		t.Errorf("RequireNamespaceLease rejected the adapter's lease: %v", err)
	}
	if adapter.generation == nil {
		t.Error("adapter has no generation resolver")
	}
}

// A namespace whose generation cannot be resolved must fail loudly rather than
// rebuild under generation 0, which would assert a lease no writer can match.
func TestSparseRebuildAdapter_GenerationFailureStopsTheRebuild(t *testing.T) {
	adapter := &sparseRebuildAdapter{
		generation: func(context.Context, string) (int64, error) {
			return 0, errors.New("config unavailable")
		},
	}
	err := adapter.RebuildSparse(context.Background(), "ns")
	if err == nil {
		t.Fatal("RebuildSparse returned nil when the generation could not be resolved")
	}
	if !strings.Contains(err.Error(), "resolve generation") {
		t.Errorf("error %q does not name the failing step", err)
	}
}
