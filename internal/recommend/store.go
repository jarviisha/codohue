package recommend

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/jarviisha/codohue/internal/core/nslifecycle"
	"github.com/jarviisha/codohue/internal/infra/metrics"
	infraqdrant "github.com/jarviisha/codohue/internal/infra/qdrant"
	infraredis "github.com/jarviisha/codohue/internal/infra/redis"
	"github.com/qdrant/go-client/qdrant"
	goredis "github.com/redis/go-redis/v9"
)

// vectorStore is the consumer-owned seam over Qdrant. The service depends on
// these domain-shaped operations instead of raw client calls, so tests fake
// one collaborator rather than a high-level fn plus the qdrant fn beneath it.
type vectorStore interface {
	// FetchSubjectVector returns the subject's sparse vector from
	// {ns}_subjects, or nil (no error) when the point or vector is absent.
	FetchSubjectVector(ctx context.Context, ns string, numericID uint64) (*qdrant.SparseVector, error)
	// FetchSubjectDenseVector returns the subject's dense embedding from
	// {ns}_subjects_dense, or nil (no error) when absent.
	FetchSubjectDenseVector(ctx context.Context, ns string, numericID uint64) ([]float32, error)
	// SearchObjects queries {ns}_objects with a sparse vector.
	SearchObjects(ctx context.Context, ns string, queryVec *qdrant.SparseVector, filter *qdrant.Filter, topK uint64) ([]*qdrant.ScoredPoint, error)
	// SearchObjectsDense queries {ns}_objects_dense with a dense vector.
	SearchObjectsDense(ctx context.Context, ns string, queryVec []float32, filter *qdrant.Filter, topK uint64) ([]*qdrant.ScoredPoint, error)
	// UpsertDensePoint writes one named dense vector plus payload into the
	// given collection under the numeric point id.
	UpsertDensePoint(ctx context.Context, collection string, numericID uint64, vector []float32, payload map[string]*qdrant.Value) error
	// DeleteFromCollection removes points by id. A missing collection is a
	// successful no-op: the object never had a vector there.
	DeleteFromCollection(ctx context.Context, collection string, ids []*qdrant.PointId) error
	// EnsureDenseCollections creates the incarnation's dense collections if
	// they do not exist yet.
	EnsureDenseCollections(ctx context.Context, inc nslifecycle.Incarnation, dim uint64, distance string) error
}

// recommendationCache is the consumer-owned seam over the Redis
// recommendation-response cache.
type recommendationCache interface {
	// Get returns the cached value; any error reads as a cache miss.
	Get(ctx context.Context, key string) (string, error)
	// Set stores the value best-effort; failures are non-fatal and silent.
	Set(ctx context.Context, key, value string, ttl time.Duration)
}

// trendingSource is the consumer-owned seam over the Redis trending ZSETs.
type trendingSource interface {
	// GetTrending reads a page of the namespace generation's trending items.
	GetTrending(ctx context.Context, ns string, generation int64, offset, limit int) ([]infraredis.TrendingEntry, error)
}

// qdrantVectorStore is the production vectorStore over *qdrant.Client.
type qdrantVectorStore struct {
	client *qdrant.Client
}

// FetchSubjectVector implements vectorStore.
func (q *qdrantVectorStore) FetchSubjectVector(ctx context.Context, ns string, numericID uint64) (*qdrant.SparseVector, error) {
	results, err := q.client.Get(ctx, &qdrant.GetPoints{
		CollectionName: ns + "_subjects",
		Ids:            []*qdrant.PointId{qdrant.NewIDNum(numericID)},
		WithVectors:    qdrant.NewWithVectorsInclude(sparseVectorName),
	})
	if err != nil {
		return nil, fmt.Errorf("get subject vector from qdrant: %w", err)
	}
	return sparseVectorOf(results), nil
}

// FetchSubjectDenseVector implements vectorStore.
func (q *qdrantVectorStore) FetchSubjectDenseVector(ctx context.Context, ns string, numericID uint64) ([]float32, error) {
	results, err := q.client.Get(ctx, &qdrant.GetPoints{
		CollectionName: ns + "_subjects_dense",
		Ids:            []*qdrant.PointId{qdrant.NewIDNum(numericID)},
		WithVectors:    qdrant.NewWithVectorsInclude(denseVectorName),
	})
	if err != nil {
		return nil, fmt.Errorf("get subject dense vector: %w", err)
	}
	return denseVectorOf(results), nil
}

// SearchObjects implements vectorStore.
func (q *qdrantVectorStore) SearchObjects(ctx context.Context, ns string, queryVec *qdrant.SparseVector, filter *qdrant.Filter, topK uint64) ([]*qdrant.ScoredPoint, error) {
	collection := ns + "_objects"
	timer := metrics.QdrantQueryDuration.WithLabelValues(ns, collection)
	start := time.Now()
	resp, err := q.client.GetPointsClient().Search(ctx, &qdrant.SearchPoints{
		CollectionName: collection,
		Vector:         queryVec.Values,
		SparseIndices:  &qdrant.SparseIndices{Data: queryVec.Indices},
		VectorName:     qdrant.PtrOf(sparseVectorName),
		Filter:         filter,
		Limit:          topK,
		WithPayload:    qdrant.NewWithPayload(true),
	})
	timer.Observe(time.Since(start).Seconds())
	if err != nil {
		return nil, fmt.Errorf("query objects from qdrant: qdrant search: %w", err)
	}
	return resp.GetResult(), nil
}

// SearchObjectsDense implements vectorStore.
func (q *qdrantVectorStore) SearchObjectsDense(ctx context.Context, ns string, queryVec []float32, filter *qdrant.Filter, topK uint64) ([]*qdrant.ScoredPoint, error) {
	collection := ns + "_objects_dense"
	start := time.Now()
	results, err := q.client.Query(ctx, &qdrant.QueryPoints{
		CollectionName: collection,
		Query:          qdrant.NewQueryDense(queryVec),
		Using:          qdrant.PtrOf(denseVectorName),
		Filter:         filter,
		Limit:          qdrant.PtrOf(topK),
		WithPayload:    qdrant.NewWithPayload(true),
	})
	metrics.QdrantQueryDuration.WithLabelValues(ns, collection).Observe(time.Since(start).Seconds())
	if err != nil {
		return nil, fmt.Errorf("query dense objects from qdrant: %w", err)
	}
	return results, nil
}

// UpsertDensePoint implements vectorStore.
func (q *qdrantVectorStore) UpsertDensePoint(ctx context.Context, collection string, numericID uint64, vector []float32, payload map[string]*qdrant.Value) error {
	_, err := q.client.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: collection,
		Points: []*qdrant.PointStruct{
			{
				Id: qdrant.NewIDNum(numericID),
				Vectors: &qdrant.Vectors{
					VectorsOptions: &qdrant.Vectors_Vectors{
						Vectors: &qdrant.NamedVectors{
							Vectors: map[string]*qdrant.Vector{
								denseVectorName: qdrant.NewVectorDense(vector),
							},
						},
					},
				},
				Payload: payload,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("qdrant upsert: %w", err)
	}
	return nil
}

// DeleteFromCollection implements vectorStore.
func (q *qdrantVectorStore) DeleteFromCollection(ctx context.Context, collection string, ids []*qdrant.PointId) error {
	_, err := q.client.Delete(ctx, &qdrant.DeletePoints{
		CollectionName: collection,
		Points: &qdrant.PointsSelector{
			PointsSelectorOneOf: &qdrant.PointsSelector_Points{
				Points: &qdrant.PointsIdsList{
					Ids: ids,
				},
			},
		},
	})
	return mapDeleteError(collection, err)
}

// EnsureDenseCollections implements vectorStore.
func (q *qdrantVectorStore) EnsureDenseCollections(ctx context.Context, inc nslifecycle.Incarnation, dim uint64, distance string) error {
	return infraqdrant.EnsureDenseCollections(ctx, q.client, inc, dim, distance)
}

// mapDeleteError treats a missing collection as a successful no-op: the object
// never had a vector there (e.g. the cron job hasn't run yet), so there is
// nothing to delete. Every other failure remains visible and retryable.
func mapDeleteError(collection string, err error) error {
	if err == nil || grpcstatus.Code(err) == codes.NotFound {
		return nil
	}
	return fmt.Errorf("delete from %q: qdrant delete: %w", collection, err)
}

// sparseVectorOf extracts the named sparse vector from a retrieved point set;
// nil when the point, the vector, or its sparse representation is absent.
func sparseVectorOf(points []*qdrant.RetrievedPoint) *qdrant.SparseVector {
	if len(points) == 0 {
		return nil
	}
	vec := points[0].GetVectors().GetVectors().GetVectors()[sparseVectorName]
	if vec == nil {
		return nil
	}
	return vec.GetSparse()
}

// denseVectorOf extracts the named dense vector's data from a retrieved point
// set; nil when the point or vector is absent.
func denseVectorOf(points []*qdrant.RetrievedPoint) []float32 {
	if len(points) == 0 {
		return nil
	}
	vec := points[0].GetVectors().GetVectors().GetVectors()[denseVectorName]
	if vec == nil {
		return nil
	}
	return vec.GetDense().GetData()
}

// redisStore is the production recommendationCache and trendingSource over
// *goredis.Client.
type redisStore struct {
	client *goredis.Client
}

// Get implements recommendationCache.
func (r *redisStore) Get(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, key).Result()
	if err != nil {
		return "", fmt.Errorf("redis cache get: %w", err)
	}
	return val, nil
}

// Set implements recommendationCache.
func (r *redisStore) Set(ctx context.Context, key, value string, ttl time.Duration) {
	r.client.Set(ctx, key, value, ttl) //nolint:errcheck // cache set is best-effort, failure is non-fatal
}

// GetTrending implements trendingSource.
func (r *redisStore) GetTrending(ctx context.Context, ns string, generation int64, offset, limit int) ([]infraredis.TrendingEntry, error) {
	return infraredis.GetTrending(ctx, r.client, ns, generation, offset, limit)
}
