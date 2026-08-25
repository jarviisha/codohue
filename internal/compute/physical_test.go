package compute

import (
	"context"
	"testing"

	"github.com/jarviisha/codohue/internal/core/nslifecycle"
	infraqdrant "github.com/jarviisha/codohue/internal/infra/qdrant"
)

func TestCollectionForContextUsesLeaseGeneration(t *testing.T) {
	if got := collectionForContext(context.Background(), "tenant", infraqdrant.CollectionObjects); got != "tenant_objects" {
		t.Fatalf("legacy collection=%q", got)
	}
	ctx := nslifecycle.ContextWithLease(context.Background(), "tenant", 3, nslifecycle.LockShared)
	if got := collectionForContext(ctx, "tenant", infraqdrant.CollectionObjectsDense); got != "tenant_g3_objects_dense" {
		t.Fatalf("generation collection=%q", got)
	}
}
