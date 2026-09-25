package compute

import (
	"context"
	"errors"
	"testing"

	"github.com/jarviisha/codohue/internal/core/nslifecycle"
	infraqdrant "github.com/jarviisha/codohue/internal/infra/qdrant"
)

func TestCollectionForContextUsesLeaseGeneration(t *testing.T) {
	ctx := nslifecycle.ContextWithLease(context.Background(), "tenant", 3, nslifecycle.LockShared)
	if got, err := collectionForContext(ctx, "tenant", infraqdrant.CollectionObjectsDense); err != nil || got != "tenant_g3_objects_dense" {
		t.Fatalf("generation collection=%q err=%v", got, err)
	}
	ctx = nslifecycle.ContextWithLease(context.Background(), "tenant", 1, nslifecycle.LockShared)
	if got, err := collectionForContext(ctx, "tenant", infraqdrant.CollectionObjects); err != nil || got != "tenant_objects" {
		t.Fatalf("legacy collection=%q err=%v", got, err)
	}
}

// leasedCtx returns a context carrying a generation-1 lease for ns — the
// write paths refuse to resolve a physical collection name without one.
func leasedCtx(ns string) context.Context {
	return nslifecycle.ContextWithLease(context.Background(), ns, 1, nslifecycle.LockShared)
}

// A missing lease must fail loudly, not silently address generation 1 —
// that would write into the collection of a deleted incarnation.
func TestCollectionForContextRequiresLease(t *testing.T) {
	if got, err := collectionForContext(context.Background(), "tenant", infraqdrant.CollectionObjects); !errors.Is(err, nslifecycle.ErrLeaseRequired) {
		t.Fatalf("expected ErrLeaseRequired, got collection=%q err=%v", got, err)
	}
}
