package main

import (
	"strings"
	"testing"

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

// A manifest row with no recorded collections contributes nothing rather than
// crashing the snapshot check.
func TestAffectedCollections_ToleratesRowsWithoutCollections(t *testing.T) {
	if got := affectedCollections(nil); got != nil {
		t.Errorf("empty manifest yielded %v", got)
	}
}
