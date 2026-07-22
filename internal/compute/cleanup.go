package compute

import (
	"context"
	"fmt"

	"github.com/qdrant/go-client/qdrant"
)

// pointScroller is the slice of *qdrant.Client the stale-point cleanup needs.
// Declared as an interface so cleanup_test.go can drive it without a live
// Qdrant.
type pointScroller interface {
	ScrollAndOffset(ctx context.Context, request *qdrant.ScrollPoints) ([]*qdrant.RetrievedPoint, *qdrant.PointId, error)
	Delete(ctx context.Context, request *qdrant.DeletePoints) (*qdrant.UpdateResult, error)
}

const cleanupScrollPageSize = uint32(1000)

// CleanupStalePoints deletes every point in collection whose numeric id is
// not in keep, returning how many were removed.
//
// The full-recompute strategy only ever upserts what the 90-day event window
// produced — without this sweep, a subject or object whose events all aged
// out keeps its last vector forever with a frozen, no-longer-decaying score,
// and keeps being surfaced by searches. An empty keep set is valid and
// empties the collection: it means nothing remains inside the window.
func CleanupStalePoints(ctx context.Context, client pointScroller, collection string, keep map[uint64]struct{}) (int, error) {
	var stale []*qdrant.PointId
	var offset *qdrant.PointId
	for {
		points, next, err := client.ScrollAndOffset(ctx, &qdrant.ScrollPoints{
			CollectionName: collection,
			Limit:          qdrant.PtrOf(cleanupScrollPageSize),
			Offset:         offset,
			WithPayload:    qdrant.NewWithPayload(false),
			WithVectors:    qdrant.NewWithVectors(false),
		})
		if err != nil {
			return 0, fmt.Errorf("scroll %s: %w", collection, err)
		}
		for _, p := range points {
			if _, ok := keep[p.GetId().GetNum()]; !ok {
				stale = append(stale, p.GetId())
			}
		}
		if next == nil || len(points) == 0 {
			break
		}
		offset = next
	}

	for start := 0; start < len(stale); start += qdrantBatchSize {
		end := min(start+qdrantBatchSize, len(stale))
		_, err := client.Delete(ctx, &qdrant.DeletePoints{
			CollectionName: collection,
			Points: &qdrant.PointsSelector{
				PointsSelectorOneOf: &qdrant.PointsSelector_Points{
					Points: &qdrant.PointsIdsList{Ids: stale[start:end]},
				},
			},
		})
		if err != nil {
			return start, fmt.Errorf("delete stale points from %s: %w", collection, err)
		}
	}
	return len(stale), nil
}

// CleanupStaleItemDensePoints removes {ns}_objects_dense points for items no
// longer produced by this run's training. Must only be called when this run
// owns the item vectors (item2vec/svd) — under dense_source="catalog" the
// collection belongs to cmd/embedder.
func CleanupStaleItemDensePoints(ctx context.Context, client pointScroller, idmapSvc idmapService, ns string, keepObjectIDs []string) (int, error) {
	keep := make(map[uint64]struct{}, len(keepObjectIDs))
	for _, id := range keepObjectIDs {
		numID, err := idmapSvc.GetOrCreateObjectID(ctx, id, ns)
		if err != nil {
			return 0, fmt.Errorf("map object id %q: %w", id, err)
		}
		keep[numID] = struct{}{}
	}
	return CleanupStalePoints(ctx, client, ns+"_objects_dense", keep)
}

// CleanupStaleSubjectDensePoints removes {ns}_subjects_dense points for
// subjects that no longer have a mean-pooled vector this run.
func CleanupStaleSubjectDensePoints(ctx context.Context, client pointScroller, idmapSvc idmapService, ns string, keepSubjectIDs []string) (int, error) {
	keep := make(map[uint64]struct{}, len(keepSubjectIDs))
	for _, id := range keepSubjectIDs {
		numID, err := idmapSvc.GetOrCreateSubjectID(ctx, id, ns)
		if err != nil {
			return 0, fmt.Errorf("map subject id %q: %w", id, err)
		}
		keep[numID] = struct{}{}
	}
	return CleanupStalePoints(ctx, client, ns+"_subjects_dense", keep)
}
