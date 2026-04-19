package qdrant

import (
	"context"
	"fmt"
	"strings"

	"github.com/qdrant/go-client/qdrant"
)

var (
	collectionExistsFn = func(ctx context.Context, client *qdrant.Client, name string) (bool, error) {
		return client.CollectionExists(ctx, name)
	}
	createCollectionFn = func(ctx context.Context, client *qdrant.Client, req *qdrant.CreateCollection) error {
		return client.CreateCollection(ctx, req)
	}
)

// EnsureCollections creates the {namespace}_subjects and {namespace}_objects sparse
// collections if they do not exist. Called at the start of each batch run before
// upserting sparse CF vectors.
func EnsureCollections(ctx context.Context, client *qdrant.Client, namespace string) error {
	for _, suffix := range []string{"subjects", "objects"} {
		name := namespace + "_" + suffix
		exists, err := collectionExistsFn(ctx, client, name)
		if err != nil {
			return fmt.Errorf("check collection %q: %w", name, err)
		}
		if exists {
			continue
		}
		if err := createSparseCollection(ctx, client, name); err != nil {
			return fmt.Errorf("create collection %q: %w", name, err)
		}
	}
	return nil
}

// EnsureDenseCollections creates the {namespace}_objects_dense and
// {namespace}_subjects_dense collections if they do not exist.
// embeddingDim is the vector dimension (e.g. 64). distance must be "cosine" or "dot".
// Called by the compute cron job before upserting dense vectors (Phase 4+).
func EnsureDenseCollections(ctx context.Context, client *qdrant.Client, namespace string, embeddingDim uint64, distance string) error {
	dist := resolveDenseDistance(distance)

	for _, suffix := range []string{"objects_dense", "subjects_dense"} {
		name := namespace + "_" + suffix
		exists, err := collectionExistsFn(ctx, client, name)
		if err != nil {
			return fmt.Errorf("check collection %q: %w", name, err)
		}
		if exists {
			continue
		}
		if err := createDenseCollection(ctx, client, name, embeddingDim, dist); err != nil {
			return fmt.Errorf("create collection %q: %w", name, err)
		}
	}
	return nil
}

func createSparseCollection(ctx context.Context, client *qdrant.Client, name string) error {
	if err := createCollectionFn(ctx, client, &qdrant.CreateCollection{
		CollectionName: name,
		SparseVectorsConfig: qdrant.NewSparseVectorsConfig(map[string]*qdrant.SparseVectorParams{
			"sparse_interactions": {
				Index: &qdrant.SparseIndexConfig{
					OnDisk: new(false),
				},
			},
		}),
	}); err != nil {
		return fmt.Errorf("create sparse collection: %w", err)
	}
	return nil
}

func createDenseCollection(ctx context.Context, client *qdrant.Client, name string, dim uint64, dist qdrant.Distance) error {
	if err := createCollectionFn(ctx, client, &qdrant.CreateCollection{
		CollectionName: name,
		// Named vector "dense_interactions" — matches the vector name used during upsert.
		VectorsConfig: qdrant.NewVectorsConfigMap(map[string]*qdrant.VectorParams{
			"dense_interactions": {
				Size:     dim,
				Distance: dist,
			},
		}),
	}); err != nil {
		return fmt.Errorf("create dense collection: %w", err)
	}
	return nil
}

// resolveDenseDistance maps the string config value to a Qdrant Distance enum.
// Defaults to Cosine for any unrecognised value.
func resolveDenseDistance(s string) qdrant.Distance {
	switch strings.ToLower(s) {
	case "dot":
		return qdrant.Distance_Dot
	default:
		return qdrant.Distance_Cosine
	}
}
