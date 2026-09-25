package compute

import (
	"context"
	"fmt"

	"github.com/jarviisha/codohue/internal/core/nslifecycle"
	infraqdrant "github.com/jarviisha/codohue/internal/infra/qdrant"
)

// collectionForContext resolves the physical collection name from the
// namespace lifecycle lease. Compute writes run under WithWriter, so a
// missing lease is an error: silently defaulting to generation 1 would
// resurrect the collection of a deleted incarnation.
func collectionForContext(ctx context.Context, namespace string, kind infraqdrant.CollectionKind) (string, error) {
	inc, ok := nslifecycle.LeaseIncarnation(ctx, namespace)
	if !ok {
		return "", fmt.Errorf("%s collection for %q: %w", kind, namespace, nslifecycle.ErrLeaseRequired)
	}
	return infraqdrant.CollectionName(inc, kind), nil
}
