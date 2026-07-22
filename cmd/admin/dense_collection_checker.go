package main

import (
	"context"
	"fmt"

	qdrantpb "github.com/qdrant/go-client/qdrant"
)

// denseCollectionChecker backs nsconfig's embedding_dim change guard.
// Collections keep their creation-time dimension and Ensure* only creates
// missing ones, so once either dense collection exists a dim change would
// fail every subsequent dense upsert.
type denseCollectionChecker struct {
	client *qdrantpb.Client
}

// DenseCollectionsExist reports whether either dense collection exists for
// the namespace.
func (c *denseCollectionChecker) DenseCollectionsExist(ctx context.Context, namespace string) (bool, error) {
	for _, name := range []string{namespace + "_objects_dense", namespace + "_subjects_dense"} {
		exists, err := c.client.CollectionExists(ctx, name)
		if err != nil {
			return false, fmt.Errorf("collection exists %s: %w", name, err)
		}
		if exists {
			return true, nil
		}
	}
	return false, nil
}
