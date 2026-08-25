package main

import (
	"context"
	"fmt"

	qdrantpb "github.com/qdrant/go-client/qdrant"

	"github.com/jarviisha/codohue/internal/compute"
	"github.com/jarviisha/codohue/internal/core/idmap"
	"github.com/jarviisha/codohue/internal/core/nslifecycle"
	infraqdrant "github.com/jarviisha/codohue/internal/infra/qdrant"
)

// The adapters here exist because internal/core/idmap must not import
// internal/compute or internal/infra/qdrant: compute already depends on idmap,
// and the repair needs to drive both. Composing them in the binary keeps the
// dependency direction intact.

// qdrantPointMover adapts the Qdrant repair primitives to idmap.PointMover.
// The numeric ids widen to uint64 here rather than in the core package, which
// has no reason to know Qdrant's id type.
type qdrantPointMover struct {
	client *qdrantpb.Client
}

func (m *qdrantPointMover) CopyPointVerified(ctx context.Context, collection string, from, to int64) error {
	return infraqdrant.CopyPointVerified(ctx, m.client, collection, uint64(from), uint64(to))
}

func (m *qdrantPointMover) DeletePoint(ctx context.Context, collection string, id int64) error {
	return infraqdrant.DeletePoint(ctx, m.client, collection, uint64(id))
}

func (m *qdrantPointMover) PointAbsent(ctx context.Context, collection string, id int64) (bool, error) {
	return infraqdrant.PointAbsent(ctx, m.client, collection, uint64(id))
}

// sparseRebuildAdapter satisfies idmap.SparseRebuilder over internal/compute.
//
// A full recompute is the only correct rebuild: sparse object vectors encode
// subject numeric ids in their coordinates, so once mappings move, the whole
// vector is stale — there is no incremental edit that fixes it.
type sparseRebuildAdapter struct {
	svc    *compute.Service
	lambda float64
}

func (a *sparseRebuildAdapter) RebuildSparse(ctx context.Context, namespace string) error {
	if _, _, err := a.svc.RecomputeNamespace(ctx, namespace, a.lambda); err != nil {
		return fmt.Errorf("recompute %q: %w", namespace, err)
	}
	return nil
}

// globalFenceAdapter satisfies idmap.GlobalFence over the lifecycle service.
// Apply runs with every writer in the fleet frozen, so nothing can mint a
// mapping or write a point while numeric ids are moving.
type globalFenceAdapter struct {
	svc *nslifecycle.Service
}

func (a *globalFenceAdapter) WithGlobalExclusive(ctx context.Context, fn func(context.Context) error) error {
	return a.svc.WithGlobalExclusive(ctx, func(locked context.Context, _ *nslifecycle.SystemLifecycle) error {
		return fn(locked)
	})
}

// repairEvidenceSource assembles the audit's raw material from both stores.
type repairEvidenceSource struct {
	repo   *idmap.RepairRepository
	qdrant *qdrantpb.Client
	// namespaces returns every configured namespace; injected so the CLI can
	// read them through nsconfig without this file importing that domain's
	// repository directly.
	namespaces func(ctx context.Context) ([]string, error)
	// generation resolves the live lifecycle generation for a namespace, so
	// the audit inspects the collections that are actually being served.
	generation func(ctx context.Context, namespace string) (int64, error)
}

func (s *repairEvidenceSource) Namespaces(ctx context.Context) ([]string, error) {
	return s.namespaces(ctx)
}

// collectionKinds pairs each collection with the entity type and payload key
// its points carry, which is how a point is tied back to a logical identity.
var collectionKinds = []struct {
	kind       infraqdrant.CollectionKind
	entityType string
	idField    string
}{
	{infraqdrant.CollectionSubjects, "subject", "subject_id"},
	{infraqdrant.CollectionSubjectsDense, "subject", "subject_id"},
	{infraqdrant.CollectionObjects, "object", "object_id"},
	{infraqdrant.CollectionObjectsDense, "object", "object_id"},
}

func (s *repairEvidenceSource) Evidence(ctx context.Context, namespace string) (*idmap.NamespaceEvidence, error) {
	mappings, err := s.repo.LoadMappings(ctx, namespace)
	if err != nil {
		return nil, err
	}
	generation, err := s.generation(ctx, namespace)
	if err != nil {
		return nil, err
	}

	evidence := &idmap.NamespaceEvidence{
		Namespace:        namespace,
		Mappings:         mappings,
		DenseCollections: map[string]bool{},
	}
	for _, spec := range collectionKinds {
		collection := infraqdrant.CollectionName(namespace, generation, spec.kind)
		points, err := infraqdrant.InventoryCollection(ctx, s.qdrant, collection, spec.idField)
		if err != nil {
			// A collection that does not exist yet is not evidence of a
			// problem — a namespace with no dense vectors simply has none.
			if infraqdrant.IsMissingCollection(err) {
				continue
			}
			return nil, err
		}
		if spec.kind == infraqdrant.CollectionSubjectsDense || spec.kind == infraqdrant.CollectionObjectsDense {
			evidence.DenseCollections[collection] = true
		}
		for _, point := range points {
			evidence.Points = append(evidence.Points, idmap.CollectionEvidence{
				Collection:  point.Collection,
				EntityType:  spec.entityType,
				NumericID:   int64(point.NumericID),
				StringID:    point.StringID,
				PayloadHash: point.PayloadHash,
				VectorHash:  point.VectorHash,
			})
		}
	}
	return evidence, nil
}

// affectedCollections lists every collection a run touches, which is the set
// that must have a recorded snapshot before apply.
func affectedCollections(items []idmap.RepairItem) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, item := range items {
		raw, ok := item.Sources["collections"].(map[string]any)
		if !ok {
			continue
		}
		for collection := range raw {
			if _, dup := seen[collection]; dup {
				continue
			}
			seen[collection] = struct{}{}
			out = append(out, collection)
		}
	}
	return out
}
