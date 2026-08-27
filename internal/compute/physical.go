package compute

import (
	"context"

	"github.com/jarviisha/codohue/internal/core/nslifecycle"
	infraqdrant "github.com/jarviisha/codohue/internal/infra/qdrant"
)

func collectionForContext(ctx context.Context, namespace string, kind infraqdrant.CollectionKind) string {
	generation, _ := nslifecycle.LeaseGeneration(ctx, namespace)
	if generation < 1 {
		generation = 1
	}
	return infraqdrant.CollectionName(namespace, generation, kind)
}
