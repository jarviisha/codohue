package compute

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qdrant/go-client/qdrant"
)

// fakeScroller pages through preset point ids and records deletions.
type fakeScroller struct {
	ids       []uint64
	pageSize  int
	scrollErr error
	deleteErr error
	deleted   []uint64
}

func (f *fakeScroller) ScrollAndOffset(_ context.Context, req *qdrant.ScrollPoints) ([]*qdrant.RetrievedPoint, *qdrant.PointId, error) {
	if f.scrollErr != nil {
		return nil, nil, f.scrollErr
	}
	start := 0
	if req.Offset != nil {
		for i, id := range f.ids {
			if id == req.Offset.GetNum() {
				start = i
				break
			}
		}
	}
	end := start + f.pageSize
	var next *qdrant.PointId
	if end >= len(f.ids) {
		end = len(f.ids)
	} else {
		next = qdrant.NewIDNum(f.ids[end])
	}
	points := make([]*qdrant.RetrievedPoint, 0, end-start)
	for _, id := range f.ids[start:end] {
		points = append(points, &qdrant.RetrievedPoint{Id: qdrant.NewIDNum(id)})
	}
	return points, next, nil
}

func (f *fakeScroller) Delete(_ context.Context, req *qdrant.DeletePoints) (*qdrant.UpdateResult, error) {
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	for _, id := range req.GetPoints().GetPoints().GetIds() {
		f.deleted = append(f.deleted, id.GetNum())
	}
	return &qdrant.UpdateResult{}, nil
}

func TestCleanupStalePoints_DeletesOnlyUnkeptAcrossPages(t *testing.T) {
	sc := &fakeScroller{ids: []uint64{1, 2, 3, 4, 5}, pageSize: 2}
	keep := map[uint64]struct{}{2: {}, 4: {}}

	n, err := CleanupStalePoints(context.Background(), sc, "ns_objects", keep)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 3 {
		t.Fatalf("removed: got %d, want 3", n)
	}
	if len(sc.deleted) != 3 {
		t.Fatalf("deleted ids: %v", sc.deleted)
	}
	for _, id := range sc.deleted {
		if id == 2 || id == 4 {
			t.Fatalf("kept point %d was deleted", id)
		}
	}
}

func TestCleanupStalePoints_EmptyKeepEmptiesCollection(t *testing.T) {
	sc := &fakeScroller{ids: []uint64{7, 8}, pageSize: 10}

	n, err := CleanupStalePoints(context.Background(), sc, "ns_subjects", map[uint64]struct{}{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 || len(sc.deleted) != 2 {
		t.Fatalf("expected the whole collection removed, got %v", sc.deleted)
	}
}

func TestCleanupStalePoints_NothingStaleNoDeletes(t *testing.T) {
	sc := &fakeScroller{ids: []uint64{1, 2}, pageSize: 10}

	n, err := CleanupStalePoints(context.Background(), sc, "ns_objects", map[uint64]struct{}{1: {}, 2: {}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 0 || len(sc.deleted) != 0 {
		t.Fatalf("expected no deletes, got %v", sc.deleted)
	}
}

func TestCleanupStalePoints_ScrollErrorPropagates(t *testing.T) {
	sc := &fakeScroller{scrollErr: errors.New("qdrant down")}
	if _, err := CleanupStalePoints(context.Background(), sc, "ns_objects", nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestRecomputeNamespace_SweepsStaleCollections(t *testing.T) {
	repo := &fakeComputeRepo{
		subjects: []string{"u1"},
		subjectEvents: map[string][]*RawEvent{
			"u1": {
				{SubjectID: "u1", ObjectID: "o1", Weight: 1, OccurredAt: time.Now().Unix()},
				{SubjectID: "u1", ObjectID: "o2", Weight: 1, OccurredAt: time.Now().Unix()},
			},
		},
	}
	svc := newTestService(repo, newFakeIDMap())
	svc.upsertFn = func(_ context.Context, _ *qdrant.UpsertPoints) error { return nil }
	cleaned := map[string]int{}
	svc.cleanupFn = func(_ context.Context, collection string, keep map[uint64]struct{}) (int, error) {
		cleaned[collection] = len(keep)
		return 0, nil
	}

	if _, _, err := svc.RecomputeNamespace(context.Background(), "ns", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, ok := cleaned["ns_subjects"]; !ok || got != 1 {
		t.Errorf("subjects sweep keep-set: got %d (called=%v), want 1", got, ok)
	}
	if got, ok := cleaned["ns_objects"]; !ok || got != 2 {
		t.Errorf("objects sweep keep-set: got %d (called=%v), want 2", got, ok)
	}
}
