package compute

import (
	"context"
	"testing"

	"github.com/jarviisha/codohue/internal/core/nslifecycle"
)

// Under dense_source="catalog" the item vectors come from cmd/embedder, and
// only the items a subject actually touched can contribute to its mean. The
// id list drives a batched Qdrant fetch, so it must be deduplicated and
// ordered — a repeated id would fetch the same point twice, and an unstable
// order makes two identical runs issue different requests.
func TestInteractedObjectIDs_DeduplicatedAndOrdered(t *testing.T) {
	events := []*RawEvent{
		{SubjectID: "u1", ObjectID: "o2"},
		{SubjectID: "u2", ObjectID: "o1"},
		{SubjectID: "u1", ObjectID: "o2"}, // repeat
		{SubjectID: "u3", ObjectID: "o3"},
	}

	got := interactedObjectIDs(events)

	want := []string{"o1", "o2", "o3"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestInteractedObjectIDs_EmptyWindow(t *testing.T) {
	if got := interactedObjectIDs(nil); len(got) != 0 {
		t.Errorf("expected no ids, got %v", got)
	}
}

// A subject's dense vector is the mean of the vectors of the items it touched,
// weighted by how often it touched them: two views of the same item pull the
// mean toward that item twice.
func TestUserDenseVectors_MeanPoolsByInteractionCount(t *testing.T) {
	itemVecs := map[string][]float32{
		"o1": {0, 4},
		"o2": {4, 0},
	}
	events := []*RawEvent{
		{SubjectID: "u1", ObjectID: "o1"},
		{SubjectID: "u1", ObjectID: "o2"},
		{SubjectID: "u2", ObjectID: "o1"},
		{SubjectID: "u2", ObjectID: "o1"}, // twice — no pull toward o2
	}

	got := UserDenseVectors(events, itemVecs)

	if len(got) != 2 {
		t.Fatalf("expected 2 subject vectors, got %d", len(got))
	}
	if v := got["u1"]; v[0] != 2 || v[1] != 2 {
		t.Errorf("u1 = %v, want the midpoint (2,2)", v)
	}
	if v := got["u2"]; v[0] != 0 || v[1] != 4 {
		t.Errorf("u2 = %v, want (0,4)", v)
	}
}

// The embedder embeds asynchronously, so a freshly-ingested item routinely has
// no vector yet. Those events must be skipped rather than pulling the mean
// toward the origin with an implicit zero vector.
func TestUserDenseVectors_SkipsItemsWithoutVectors(t *testing.T) {
	itemVecs := map[string][]float32{"embedded": {2, 2}}
	events := []*RawEvent{
		{SubjectID: "u1", ObjectID: "embedded"},
		{SubjectID: "u1", ObjectID: "not-embedded-yet"},
	}

	got := UserDenseVectors(events, itemVecs)

	if v := got["u1"]; len(v) != 2 || v[0] != 2 || v[1] != 2 {
		t.Errorf("u1 = %v, want (2,2): an unembedded item must not count toward the mean", v)
	}
}

// A subject whose every interacted item is still unembedded has no derivable
// vector. Omitting it leaves the previous vector in place for the cleanup
// sweep to judge, rather than writing a meaningless zero vector.
func TestUserDenseVectors_SubjectWithNoEmbeddedItemsIsOmitted(t *testing.T) {
	got := UserDenseVectors(
		[]*RawEvent{{SubjectID: "u1", ObjectID: "not-embedded-yet"}},
		map[string][]float32{"other": {1, 1}},
	)

	if _, ok := got["u1"]; ok {
		t.Errorf("expected u1 to be omitted, got %v", got)
	}
}

func TestUserDenseVectors_NoItemVectorsAtAll(t *testing.T) {
	if got := UserDenseVectors([]*RawEvent{{SubjectID: "u1", ObjectID: "o1"}}, nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// The dense cleanup helpers target the generation the run holds a lease for.
// Sweeping generation 1's collections during a generation-3 run would delete
// the live namespace's vectors.
func TestDenseCleanupHelpers_TargetLeaseGenerationCollections(t *testing.T) {
	for _, tc := range []struct {
		name    string
		cleanup func(context.Context, pointScroller) (int, error)
		want    string
	}{
		{
			name: "item dense",
			cleanup: func(ctx context.Context, sc pointScroller) (int, error) {
				return CleanupStaleItemDensePoints(ctx, sc, newFakeIDMap(), "ns", nil)
			},
			want: "ns_g3_objects_dense",
		},
		{
			name: "subject dense",
			cleanup: func(ctx context.Context, sc pointScroller) (int, error) {
				return CleanupStaleSubjectDensePoints(ctx, sc, newFakeIDMap(), "ns", nil)
			},
			want: "ns_g3_subjects_dense",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sc := &fakeScroller{ids: []uint64{1}, pageSize: 10}
			ctx := nslifecycle.ContextWithLease(context.Background(), "ns", 3, nslifecycle.LockShared)

			// An empty keep set is authoritative: it means nothing survives the
			// window, so the whole collection goes.
			n, err := tc.cleanup(ctx, sc)
			if err != nil {
				t.Fatalf("cleanup: %v", err)
			}
			if n != 1 {
				t.Errorf("removed %d points, want 1", n)
			}
			if sc.lastCollection != tc.want {
				t.Errorf("swept %q, want %q", sc.lastCollection, tc.want)
			}
		})
	}
}
