package recommend

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"time"

	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/jarviisha/codohue/internal/core/idmap"
	"github.com/jarviisha/codohue/internal/infra/metrics"
	infraqdrant "github.com/jarviisha/codohue/internal/infra/qdrant"
	infraredis "github.com/jarviisha/codohue/internal/infra/redis"
	"github.com/jarviisha/codohue/internal/nsconfig"
	"github.com/qdrant/go-client/qdrant"
	goredis "github.com/redis/go-redis/v9"
)

const (
	coldStartThreshold   = 5
	defaultGamma         = 0.02 // freshness decay per day for objects
	recCacheTTL          = 5 * time.Minute
	sparseVectorName     = "sparse_interactions"
	denseVectorName      = "dense_interactions"
	cfOverFetchFactor    = 5
	denseOverFetchFactor = 3
	normEpsilon          = 1e-9
)

type recommendRepo interface {
	CountInteractions(ctx context.Context, namespace, subjectID string) (int, error)
	GetSeenItems(ctx context.Context, namespace, subjectID string, seenItemsDays int) ([]string, error)
	GetPopularItems(ctx context.Context, namespace string, limit int) ([]string, error)
}

type recommendNsConfig interface {
	Get(ctx context.Context, namespace string) (*nsconfig.NamespaceConfig, error)
}

type recommendIDMapper interface {
	GetOrCreateSubjectID(ctx context.Context, subjectID, namespace string) (uint64, error)
	GetOrCreateObjectID(ctx context.Context, objectID, namespace string) (uint64, error)
}

// Service serves recommendations via collaborative filtering or fallback to popular items.
type Service struct {
	repo        recommendRepo
	nsConfigSvc recommendNsConfig
	idmapSvc    recommendIDMapper
	qdrant      *qdrant.Client

	// injectable for testing — wired to real implementations in NewService
	getCacheFn               func(ctx context.Context, key string) (string, error)
	setCacheFn               func(ctx context.Context, key, value string, ttl time.Duration)
	getTrendingFn            func(ctx context.Context, ns string, offset, limit int) ([]infraredis.TrendingEntry, error)
	fetchSubjectVecFn        func(ctx context.Context, ns string, numID uint64) (*qdrant.SparseVector, error)
	fetchSubjectDenseVecFn   func(ctx context.Context, ns string, numID uint64) ([]float32, error)
	searchObjectsFn          func(ctx context.Context, namespace string, queryVec *qdrant.SparseVector, filter *qdrant.Filter, topK uint64) ([]*qdrant.ScoredPoint, error)
	searchObjectsDenseFn     func(ctx context.Context, namespace string, queryVec []float32, filter *qdrant.Filter, topK uint64) ([]*qdrant.ScoredPoint, error)
	deleteFromCollectionFn   func(ctx context.Context, collection string, ids []*qdrant.PointId) error
	ensureDenseCollectionsFn func(ctx context.Context, ns string, dim uint64, distance string) error
	qdrantGetFn              func(ctx context.Context, points *qdrant.GetPoints) ([]*qdrant.RetrievedPoint, error)
	qdrantSearchFn           func(ctx context.Context, points *qdrant.SearchPoints) ([]*qdrant.ScoredPoint, error)
	qdrantQueryFn            func(ctx context.Context, points *qdrant.QueryPoints) ([]*qdrant.ScoredPoint, error)
	qdrantUpsertFn           func(ctx context.Context, points *qdrant.UpsertPoints) error
	qdrantDeleteFn           func(ctx context.Context, points *qdrant.DeletePoints) error
}

// NewService creates a new Service with all required dependencies.
func NewService(
	repo *Repository,
	nsConfigSvc *nsconfig.Service,
	idmapSvc *idmap.Service,
	qdrantClient *qdrant.Client,
	redisClient *goredis.Client,
) *Service {
	s := &Service{
		repo:        repo,
		nsConfigSvc: nsConfigSvc,
		idmapSvc:    idmapSvc,
		qdrant:      qdrantClient,
	}
	s.getCacheFn = func(ctx context.Context, key string) (string, error) {
		return redisClient.Get(ctx, key).Result()
	}
	s.setCacheFn = func(ctx context.Context, key, value string, ttl time.Duration) {
		redisClient.Set(ctx, key, value, ttl) //nolint:errcheck // cache set is best-effort, failure is non-fatal
	}
	s.getTrendingFn = func(ctx context.Context, ns string, offset, limit int) ([]infraredis.TrendingEntry, error) {
		return infraredis.GetTrending(ctx, redisClient, ns, offset, limit)
	}
	s.fetchSubjectVecFn = s.fetchSubjectVector
	s.fetchSubjectDenseVecFn = s.fetchSubjectDenseVector
	s.searchObjectsFn = s.searchObjects
	s.searchObjectsDenseFn = s.searchObjectsDense
	s.deleteFromCollectionFn = s.deleteFromCollection
	s.qdrantGetFn = func(ctx context.Context, points *qdrant.GetPoints) ([]*qdrant.RetrievedPoint, error) {
		return qdrantClient.Get(ctx, points)
	}
	s.qdrantSearchFn = func(ctx context.Context, points *qdrant.SearchPoints) ([]*qdrant.ScoredPoint, error) {
		resp, err := qdrantClient.GetPointsClient().Search(ctx, points)
		if err != nil {
			return nil, fmt.Errorf("qdrant search: %w", err)
		}
		return resp.GetResult(), nil
	}
	s.qdrantQueryFn = func(ctx context.Context, points *qdrant.QueryPoints) ([]*qdrant.ScoredPoint, error) {
		return qdrantClient.Query(ctx, points)
	}
	s.qdrantUpsertFn = func(ctx context.Context, points *qdrant.UpsertPoints) error {
		_, err := qdrantClient.Upsert(ctx, points)
		if err != nil {
			return fmt.Errorf("qdrant upsert: %w", err)
		}
		return nil
	}
	s.qdrantDeleteFn = func(ctx context.Context, points *qdrant.DeletePoints) error {
		_, err := qdrantClient.Delete(ctx, points)
		if err != nil {
			return fmt.Errorf("qdrant delete: %w", err)
		}
		return nil
	}
	s.ensureDenseCollectionsFn = func(ctx context.Context, ns string, dim uint64, distance string) error {
		return infraqdrant.EnsureDenseCollections(ctx, qdrantClient, ns, dim, distance)
	}
	return s
}

// StoreObjectEmbedding stores a BYOE dense vector for an object in {ns}_objects_dense.
func (s *Service) StoreObjectEmbedding(ctx context.Context, namespace, objectID string, vector []float32) error {
	return s.storeEmbedding(ctx, namespace, objectID, "object", vector)
}

// StoreSubjectEmbedding stores a BYOE dense vector for a subject in {ns}_subjects_dense.
func (s *Service) StoreSubjectEmbedding(ctx context.Context, namespace, subjectID string, vector []float32) error {
	return s.storeEmbedding(ctx, namespace, subjectID, "subject", vector)
}

func (s *Service) storeEmbedding(ctx context.Context, namespace, entityID, entityType string, vector []float32) error {
	cfg, err := s.nsConfigSvc.Get(ctx, namespace)
	if err != nil {
		return fmt.Errorf("get ns config: %w", err)
	}

	// Validate dimension when config is present.
	if cfg != nil && cfg.EmbeddingDim > 0 && len(vector) != cfg.EmbeddingDim {
		return fmt.Errorf("embedding dimension mismatch: got %d, want %d", len(vector), cfg.EmbeddingDim)
	}

	// Ensure both dense collections exist. The cron job normally creates them, but BYOE
	// endpoints must work independently of whether the batch job has run for this namespace.
	dim := uint64(len(vector))
	distance := "cosine"
	if cfg != nil {
		if cfg.EmbeddingDim > 0 {
			dim = uint64(cfg.EmbeddingDim)
		}
		if cfg.DenseDistance != "" {
			distance = cfg.DenseDistance
		}
	}
	if err := s.ensureDenseCollectionsFn(ctx, namespace, dim, distance); err != nil {
		return fmt.Errorf("ensure dense collections: %w", err)
	}

	// Resolve collection name.
	collection := namespace + "_" + entityType + "s_dense"
	idKey := entityType + "_id"

	// Get or create numeric ID.
	var numID uint64
	if entityType == "object" {
		numID, err = s.idmapSvc.GetOrCreateObjectID(ctx, entityID, namespace)
	} else {
		numID, err = s.idmapSvc.GetOrCreateSubjectID(ctx, entityID, namespace)
	}
	if err != nil {
		return fmt.Errorf("get numeric id: %w", err)
	}

	err = s.qdrantUpsertFn(ctx, &qdrant.UpsertPoints{
		CollectionName: collection,
		Points: []*qdrant.PointStruct{
			{
				Id: qdrant.NewIDNum(numID),
				Vectors: &qdrant.Vectors{
					VectorsOptions: &qdrant.Vectors_Vectors{
						Vectors: &qdrant.NamedVectors{
							Vectors: map[string]*qdrant.Vector{
								denseVectorName: qdrant.NewVectorDense(vector),
							},
						},
					},
				},
				Payload: map[string]*qdrant.Value{
					idKey:        qdrant.NewValueString(entityID),
					"strategy":   qdrant.NewValueString("byoe"),
					"updated_at": qdrant.NewValueString(time.Now().UTC().Format(time.RFC3339)),
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("upsert dense vector: %w", err)
	}
	return nil
}

// Recommend returns recommended items for a subject, selecting the strategy based on interaction history.
func (s *Service) Recommend(ctx context.Context, req *Request) (*Response, error) {
	if req.Limit <= 0 {
		req.Limit = 20
	}

	cfg, err := s.nsConfigSvc.Get(ctx, req.Namespace)
	if err != nil {
		slog.Error("get ns config failed", "namespace", req.Namespace, "error", err)
	}
	maxResults := req.Limit
	if cfg != nil && cfg.MaxResults > 0 && cfg.MaxResults < maxResults {
		maxResults = cfg.MaxResults
	}

	cacheKey := recCacheKey(req.Namespace, req.SubjectID, maxResults)
	if cached, err := s.getCacheFn(ctx, cacheKey); err == nil {
		var resp Response
		if json.Unmarshal([]byte(cached), &resp) == nil {
			metrics.RedisCacheRequests.WithLabelValues("hit").Inc()
			return &resp, nil
		}
	}
	metrics.RedisCacheRequests.WithLabelValues("miss").Inc()

	resp, err := s.doRecommend(ctx, req, maxResults, cfg)
	if err != nil {
		return nil, err
	}

	if b, err := json.Marshal(resp); err == nil {
		s.setCacheFn(ctx, cacheKey, string(b), recCacheTTL)
	}
	return resp, nil
}

func (s *Service) doRecommend(ctx context.Context, req *Request, maxResults int, cfg *nsconfig.NamespaceConfig) (*Response, error) {
	count, err := s.repo.CountInteractions(ctx, req.Namespace, req.SubjectID)
	if err != nil {
		slog.Error("count interactions failed", "namespace", req.Namespace, "subject_id", req.SubjectID, "error", err)
	}

	if count == 0 {
		return s.fallbackTrending(ctx, req, maxResults, cfg)
	}
	if count < coldStartThreshold {
		return s.hybridCold(ctx, req, maxResults, cfg)
	}
	return s.collaborativeFiltering(ctx, req, maxResults, cfg)
}

func (s *Service) collaborativeFiltering(ctx context.Context, req *Request, limit int, cfg *nsconfig.NamespaceConfig) (*Response, error) {
	subjectNumID, err := s.idmapSvc.GetOrCreateSubjectID(ctx, req.SubjectID, req.Namespace)
	if err != nil {
		slog.Error("get subject numeric id failed", "namespace", req.Namespace, "subject_id", req.SubjectID, "error", err)
		return s.fallbackPopular(ctx, req, limit)
	}

	subjectVec, err := s.fetchSubjectVecFn(ctx, req.Namespace, subjectNumID)
	if err != nil || subjectVec == nil {
		slog.Error("fetch subject vector failed", "namespace", req.Namespace, "subject_id", req.SubjectID, "error", err)
		return s.fallbackPopular(ctx, req, limit)
	}

	seenItemsDays := 30
	if cfg != nil && cfg.SeenItemsDays > 0 {
		seenItemsDays = cfg.SeenItemsDays
	}
	seenItems, err := s.repo.GetSeenItems(ctx, req.Namespace, req.SubjectID, seenItemsDays)
	if err != nil {
		slog.Error("get seen items failed", "namespace", req.Namespace, "subject_id", req.SubjectID, "error", err)
	}
	seenFilter := s.buildSeenItemsFilter(ctx, req.Namespace, seenItems)

	// Use hybrid scoring when alpha < 1.0 and dense strategy is active.
	// Note: subject dense vectors are computed during the cron batch run and may be up
	// to one cron interval stale. New interactions since the last batch are not reflected
	// in the dense component. The sparse CF component is unaffected — it queries Qdrant
	// against vectors recomputed in the same batch. To reduce staleness, decrease
	// BATCH_INTERVAL_MINUTES or push subject embeddings via BYOE after each interaction.
	if cfg != nil && cfg.Alpha > 0 && cfg.Alpha < 1.0 && cfg.DenseStrategy != "" && cfg.DenseStrategy != "disabled" {
		denseVec, err := s.fetchSubjectDenseVecFn(ctx, req.Namespace, subjectNumID)
		if err == nil && denseVec != nil {
			return s.hybridRecommend(ctx, req, limit, cfg, subjectVec, denseVec, seenFilter)
		}
		// Subject has no dense vector yet — fall through to pure sparse CF.
		slog.Debug("hybrid: no subject dense vector, using pure CF", "namespace", req.Namespace, "subject_id", req.SubjectID)
	}

	results, err := s.searchObjectsFn(ctx, req.Namespace, subjectVec, seenFilter, uint64(limit*cfOverFetchFactor))
	if err != nil {
		slog.Error("search objects failed", "namespace", req.Namespace, "subject_id", req.SubjectID, "error", err)
		return s.fallbackPopular(ctx, req, limit)
	}

	gamma := defaultGamma
	if cfg != nil && cfg.Gamma > 0 {
		gamma = cfg.Gamma
	}
	items := rerank(results, gamma, limit)

	metrics.RecommendRequests.WithLabelValues(req.Namespace, SourceCollaborativeFiltering).Inc()
	return &Response{
		SubjectID:   req.SubjectID,
		Namespace:   req.Namespace,
		Items:       items,
		Source:      SourceCollaborativeFiltering,
		GeneratedAt: time.Now().UTC(),
	}, nil
}

// hybridRecommend performs hybrid retrieval (sparse + dense) and blends scores.
func (s *Service) hybridRecommend(
	ctx context.Context,
	req *Request,
	limit int,
	cfg *nsconfig.NamespaceConfig,
	subjectSparseVec *qdrant.SparseVector,
	subjectDenseVec []float32,
	seenFilter *qdrant.Filter,
) (*Response, error) {
	alpha := cfg.Alpha

	// Sparse retrieval.
	sparseResults, err := s.searchObjectsFn(ctx, req.Namespace, subjectSparseVec, seenFilter, uint64(limit*cfOverFetchFactor))
	if err != nil {
		slog.Error("hybrid: sparse search failed", "namespace", req.Namespace, "error", err)
		sparseResults = nil
	}

	// Dense retrieval.
	denseResults, err := s.searchObjectsDenseFn(ctx, req.Namespace, subjectDenseVec, seenFilter, uint64(limit*denseOverFetchFactor))
	if err != nil {
		slog.Error("hybrid: dense search failed", "namespace", req.Namespace, "error", err)
		denseResults = nil
	}

	if len(sparseResults) == 0 && len(denseResults) == 0 {
		return s.fallbackPopular(ctx, req, limit)
	}

	// Build per-item score maps.
	sparseScores := extractScores(sparseResults)
	denseScores := extractScores(denseResults)

	// Normalize each score set independently.
	normSparse := normalizeScores(sparseScores)
	normDense := normalizeScores(denseScores)

	// Collect all candidate object IDs.
	candidateSet := make(map[string]struct{}, len(normSparse)+len(normDense))
	for id := range normSparse {
		candidateSet[id] = struct{}{}
	}
	for id := range normDense {
		candidateSet[id] = struct{}{}
	}

	// Blend scores and apply time decay.
	gamma := defaultGamma
	if cfg.Gamma > 0 {
		gamma = cfg.Gamma
	}
	now := time.Now().UTC()

	type candidate struct {
		objectID string
		score    float64
	}
	candidates := make([]candidate, 0, len(candidateSet))

	// Build lookup for created_at from Qdrant payload.
	createdAt := buildCreatedAtLookup(sparseResults, denseResults)

	for objectID := range candidateSet {
		sp := normSparse[objectID]
		dp := normDense[objectID]
		blended := alpha*sp + (1-alpha)*dp

		// Apply freshness decay if created_at is available.
		if t, ok := createdAt[objectID]; ok {
			daysSince := now.Sub(t).Hours() / 24
			blended *= math.Exp(-gamma * daysSince)
		}
		candidates = append(candidates, candidate{objectID: objectID, score: blended})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	top := limit
	if top > len(candidates) {
		top = len(candidates)
	}
	items := make([]string, top)
	for i, c := range candidates[:top] {
		items[i] = c.objectID
	}

	metrics.RecommendRequests.WithLabelValues(req.Namespace, SourceHybrid).Inc()
	return &Response{
		SubjectID:   req.SubjectID,
		Namespace:   req.Namespace,
		Items:       items,
		Source:      SourceHybrid,
		GeneratedAt: time.Now().UTC(),
	}, nil
}

// fetchSubjectDenseVector retrieves the dense embedding for a subject from {ns}_subjects_dense.
func (s *Service) fetchSubjectDenseVector(ctx context.Context, namespace string, numericID uint64) ([]float32, error) {
	results, err := s.qdrantGetFn(ctx, &qdrant.GetPoints{
		CollectionName: namespace + "_subjects_dense",
		Ids:            []*qdrant.PointId{qdrant.NewIDNum(numericID)},
		WithVectors:    qdrant.NewWithVectorsInclude(denseVectorName),
	})
	if err != nil {
		return nil, fmt.Errorf("get subject dense vector: %w", err)
	}
	if len(results) == 0 {
		return nil, nil
	}
	vec := results[0].GetVectors().GetVectors().GetVectors()[denseVectorName]
	if vec == nil {
		return nil, nil
	}
	return vec.GetDense().GetData(), nil
}

// searchObjectsDense queries {ns}_objects_dense using a dense vector.
func (s *Service) searchObjectsDense(ctx context.Context, namespace string, queryVec []float32, filter *qdrant.Filter, topK uint64) ([]*qdrant.ScoredPoint, error) {
	collection := namespace + "_objects_dense"
	start := time.Now()
	results, err := s.qdrantQueryFn(ctx, &qdrant.QueryPoints{
		CollectionName: collection,
		Query:          qdrant.NewQueryDense(queryVec),
		Using:          qdrant.PtrOf(denseVectorName),
		Filter:         filter,
		Limit:          qdrant.PtrOf(topK),
		WithPayload:    qdrant.NewWithPayload(true),
	})
	metrics.QdrantQueryDuration.WithLabelValues(namespace, collection).Observe(time.Since(start).Seconds())
	if err != nil {
		return nil, fmt.Errorf("query dense objects from qdrant: %w", err)
	}
	return results, nil
}

// extractScores builds an objectID → raw score map from Qdrant results.
func extractScores(points []*qdrant.ScoredPoint) map[string]float64 {
	m := make(map[string]float64, len(points))
	for _, p := range points {
		if v, ok := p.Payload["object_id"]; ok {
			m[v.GetStringValue()] = float64(p.Score)
		}
	}
	return m
}

// normalizeScores applies min-max normalization to a score map.
// When all scores are equal (max == min), every item receives 1.0.
func normalizeScores(scores map[string]float64) map[string]float64 {
	if len(scores) == 0 {
		return scores
	}
	var mn, mx float64
	first := true
	for _, v := range scores {
		switch {
		case first:
			mn, mx = v, v
			first = false
		case v < mn:
			mn = v
		case v > mx:
			mx = v
		}
	}
	rng := mx - mn
	result := make(map[string]float64, len(scores))
	for id, v := range scores {
		if rng < normEpsilon {
			result[id] = 1.0
		} else {
			result[id] = (v - mn) / (rng + normEpsilon)
		}
	}
	return result
}

// buildCreatedAtLookup extracts created_at timestamps from Qdrant payloads.
func buildCreatedAtLookup(sets ...[]*qdrant.ScoredPoint) map[string]time.Time {
	m := make(map[string]time.Time)
	for _, pts := range sets {
		for _, p := range pts {
			objVal, ok := p.Payload["object_id"]
			if !ok {
				continue
			}
			id := objVal.GetStringValue()
			if _, seen := m[id]; seen {
				continue
			}
			if tVal, ok := p.Payload["created_at"]; ok {
				if t, err := time.Parse(time.RFC3339, tVal.GetStringValue()); err == nil {
					m[id] = t
				}
			}
		}
	}
	return m
}

func (s *Service) hybridCold(ctx context.Context, req *Request, limit int, cfg *nsconfig.NamespaceConfig) (*Response, error) {
	cfResp, cfErr := s.collaborativeFiltering(ctx, req, limit, cfg)
	popularResp, popErr := s.fallbackTrending(ctx, req, limit, cfg)

	if popErr != nil && cfErr != nil {
		return nil, fmt.Errorf("hybrid cold: popular: %w; cf: %v", popErr, cfErr)
	}
	if popErr != nil {
		if cfResp != nil {
			cfResp.Source = SourceHybridCold
		}
		return cfResp, nil
	}
	if cfErr != nil || len(cfResp.Items) == 0 {
		return popularResp, nil
	}

	blended := blendItems(popularResp.Items, cfResp.Items, 0.7, limit)
	metrics.RecommendRequests.WithLabelValues(req.Namespace, SourceHybridCold).Inc()
	return &Response{
		SubjectID:   req.SubjectID,
		Namespace:   req.Namespace,
		Items:       blended,
		Source:      SourceHybridCold,
		GeneratedAt: time.Now().UTC(),
	}, nil
}

// GetTrending returns the trending items for a namespace from Redis.
// windowHours overrides the namespace config when > 0.
func (s *Service) GetTrending(ctx context.Context, ns string, limit, offset, windowHours int) (*TrendingResponse, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	cfg, err := s.nsConfigSvc.Get(ctx, ns)
	if err != nil {
		slog.Error("get trending ns config", "namespace", ns, "error", err)
	}

	actualWindow := 24
	if windowHours > 0 {
		actualWindow = windowHours
	} else if cfg != nil && cfg.TrendingWindow > 0 {
		actualWindow = cfg.TrendingWindow
	}

	entries, err := s.getTrendingFn(ctx, ns, offset, limit)
	if err != nil {
		slog.Error("get trending from redis", "namespace", ns, "error", err)
		entries = nil
	}

	items := make([]TrendingItem, len(entries))
	for i, e := range entries {
		items[i] = TrendingItem{ObjectID: e.ObjectID, Score: e.Score}
	}

	metrics.TrendingRequestsTotal.WithLabelValues(ns).Inc()
	return &TrendingResponse{
		Namespace:   ns,
		Items:       items,
		WindowHours: actualWindow,
		GeneratedAt: time.Now().UTC(),
	}, nil
}

// fallbackTrending serves trending items from Redis as the cold-start response.
// When the trending cache is empty or unavailable, falls back to DB popular items.
func (s *Service) fallbackTrending(ctx context.Context, req *Request, limit int, cfg *nsconfig.NamespaceConfig) (*Response, error) {
	entries, err := s.getTrendingFn(ctx, req.Namespace, 0, limit)
	if err == nil && len(entries) > 0 {
		items := make([]string, len(entries))
		for i, e := range entries {
			items[i] = e.ObjectID
		}
		metrics.RecommendRequests.WithLabelValues(req.Namespace, SourceFallbackPopular).Inc()
		return &Response{
			SubjectID:   req.SubjectID,
			Namespace:   req.Namespace,
			Items:       items,
			Source:      SourceFallbackPopular,
			GeneratedAt: time.Now().UTC(),
		}, nil
	}
	// Trending cache empty or unavailable — fall back to DB popular items.
	return s.fallbackPopular(ctx, req, limit)
}

func (s *Service) fallbackPopular(ctx context.Context, req *Request, limit int) (*Response, error) {
	items, err := s.repo.GetPopularItems(ctx, req.Namespace, limit)
	if err != nil {
		return nil, fmt.Errorf("get popular items: %w", err)
	}
	metrics.RecommendRequests.WithLabelValues(req.Namespace, SourceFallbackPopular).Inc()
	return &Response{
		SubjectID:   req.SubjectID,
		Namespace:   req.Namespace,
		Items:       items,
		Source:      SourceFallbackPopular,
		GeneratedAt: time.Now().UTC(),
	}, nil
}

func (s *Service) fetchSubjectVector(ctx context.Context, namespace string, numericID uint64) (*qdrant.SparseVector, error) {
	results, err := s.qdrantGetFn(ctx, &qdrant.GetPoints{
		CollectionName: namespace + "_subjects",
		Ids:            []*qdrant.PointId{qdrant.NewIDNum(numericID)},
		WithVectors:    qdrant.NewWithVectorsInclude(sparseVectorName),
	})
	if err != nil {
		return nil, fmt.Errorf("get subject vector from qdrant: %w", err)
	}
	if len(results) == 0 {
		return nil, nil
	}

	vecOutput := results[0].GetVectors().GetVectors().GetVectors()[sparseVectorName]
	if vecOutput == nil {
		return nil, nil
	}
	return vecOutput.GetSparse(), nil
}

func (s *Service) searchObjects(ctx context.Context, namespace string, queryVec *qdrant.SparseVector, filter *qdrant.Filter, topK uint64) ([]*qdrant.ScoredPoint, error) {
	collection := namespace + "_objects"
	timer := metrics.QdrantQueryDuration.WithLabelValues(namespace, collection)
	start := time.Now()
	results, err := s.qdrantSearchFn(ctx, &qdrant.SearchPoints{
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
		return nil, fmt.Errorf("query objects from qdrant: %w", err)
	}
	return results, nil
}

func (s *Service) buildSeenItemsFilter(ctx context.Context, namespace string, seenStringIDs []string) *qdrant.Filter {
	if len(seenStringIDs) == 0 {
		return nil
	}
	ids := make([]*qdrant.PointId, 0, len(seenStringIDs))
	for _, sid := range seenStringIDs {
		numID, err := s.idmapSvc.GetOrCreateObjectID(ctx, sid, namespace)
		if err != nil {
			continue
		}
		ids = append(ids, qdrant.NewIDNum(numID))
	}
	if len(ids) == 0 {
		return nil
	}
	return &qdrant.Filter{
		MustNot: []*qdrant.Condition{
			qdrant.NewHasID(ids...),
		},
	}
}

type scoredItem struct {
	objectID   string
	finalScore float64
}

// rerankScored applies gamma freshness decay to scored Qdrant points and returns
// the top-limit items sorted by final score descending.
func rerankScored(points []*qdrant.ScoredPoint, gamma float64, limit int) []scoredItem {
	now := time.Now().UTC()
	scored := make([]scoredItem, 0, len(points))

	for _, p := range points {
		objVal, ok := p.Payload["object_id"]
		if !ok {
			continue
		}
		finalScore := float64(p.Score)

		if createdAtVal, ok := p.Payload["created_at"]; ok {
			if t, err := time.Parse(time.RFC3339, createdAtVal.GetStringValue()); err == nil {
				daysSince := now.Sub(t).Hours() / 24
				finalScore *= math.Exp(-gamma * daysSince)
			}
		}
		scored = append(scored, scoredItem{objectID: objVal.GetStringValue(), finalScore: finalScore})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].finalScore > scored[j].finalScore
	})

	if limit < len(scored) {
		scored = scored[:limit]
	}
	return scored
}

// rerank is a convenience wrapper around rerankScored that returns only object IDs.
// Used by recommendation paths that do not need per-item scores.
func rerank(points []*qdrant.ScoredPoint, gamma float64, limit int) []string {
	scored := rerankScored(points, gamma, limit)
	result := make([]string, len(scored))
	for i, s := range scored {
		result[i] = s.objectID
	}
	return result
}

// blendItems interleaves popular and cf items at the given popularRatio, deduplicating.
func blendItems(popular, cf []string, popularRatio float64, limit int) []string {
	popularCount := int(math.Round(float64(limit) * popularRatio))
	cfCount := limit - popularCount

	seen := make(map[string]bool)
	result := make([]string, 0, limit)

	take := func(items []string, n int) {
		for _, item := range items {
			if len(result) >= limit {
				return
			}
			if n <= 0 {
				return
			}
			if !seen[item] {
				seen[item] = true
				result = append(result, item)
				n--
			}
		}
	}

	take(popular, popularCount)
	take(cf, cfCount)

	// Fill remaining slots if either list was short
	take(popular, limit-len(result))
	take(cf, limit-len(result))

	return result
}

// Rank scores a list of candidate items for a subject using sparse CF vectors
// and returns them in descending score order. If the subject has no interaction
// history, candidates are returned in their original order.
func (s *Service) Rank(ctx context.Context, req *RankRequest) (*RankResponse, error) {
	if len(req.Candidates) == 0 {
		return &RankResponse{
			SubjectID:   req.SubjectID,
			Namespace:   req.Namespace,
			Items:       []RankedItem{},
			Source:      SourceHybridRank,
			GeneratedAt: time.Now().UTC(),
		}, nil
	}

	cfg, err := s.nsConfigSvc.Get(ctx, req.Namespace)
	if err != nil {
		slog.Error("rank: get ns config failed", "namespace", req.Namespace, "error", err)
	}

	subjectNumID, err := s.idmapSvc.GetOrCreateSubjectID(ctx, req.SubjectID, req.Namespace)
	if err != nil {
		slog.Error("rank: get subject numeric id failed", "namespace", req.Namespace, "subject_id", req.SubjectID, "error", err)
		return s.rankFallback(req), nil
	}

	subjectVec, err := s.fetchSubjectVecFn(ctx, req.Namespace, subjectNumID)
	if err != nil || subjectVec == nil {
		slog.Info("rank: no subject vector, returning original order", "namespace", req.Namespace, "subject_id", req.SubjectID)
		return s.rankFallback(req), nil
	}

	ids := make([]*qdrant.PointId, 0, len(req.Candidates))
	for _, candidateID := range req.Candidates {
		numID, err := s.idmapSvc.GetOrCreateObjectID(ctx, candidateID, req.Namespace)
		if err != nil {
			slog.Error("rank: get object numeric id failed", "object_id", candidateID, "error", err)
			continue
		}
		ids = append(ids, qdrant.NewIDNum(numID))
	}

	if len(ids) == 0 {
		return s.rankFallback(req), nil
	}

	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			qdrant.NewHasID(ids...),
		},
	}

	results, err := s.searchObjectsFn(ctx, req.Namespace, subjectVec, filter, uint64(len(ids)))
	if err != nil {
		slog.Error("rank: search objects failed", "namespace", req.Namespace, "subject_id", req.SubjectID, "error", err)
		return s.rankFallback(req), nil
	}

	gamma := defaultGamma
	if cfg != nil && cfg.Gamma > 0 {
		gamma = cfg.Gamma
	}
	scored := rerankScored(results, gamma, len(req.Candidates))

	ranked := make([]RankedItem, len(scored))
	for i, s := range scored {
		ranked[i] = RankedItem{ObjectID: s.objectID, Score: s.finalScore}
	}

	metrics.RecommendRequests.WithLabelValues(req.Namespace, SourceHybridRank).Inc()
	return &RankResponse{
		SubjectID:   req.SubjectID,
		Namespace:   req.Namespace,
		Items:       ranked,
		Source:      SourceHybridRank,
		GeneratedAt: time.Now().UTC(),
	}, nil
}

// rankFallback returns candidates in their original order when CF scoring is unavailable.
// Score is set to 0 to signal to callers that no relevance information is available.
func (s *Service) rankFallback(req *RankRequest) *RankResponse {
	items := make([]RankedItem, len(req.Candidates))
	for i, c := range req.Candidates {
		items[i] = RankedItem{ObjectID: c, Score: 0}
	}
	return &RankResponse{
		SubjectID:   req.SubjectID,
		Namespace:   req.Namespace,
		Items:       items,
		Source:      SourceHybridRank,
		GeneratedAt: time.Now().UTC(),
	}
}

// DeleteObject removes an object from all Qdrant collections for the given namespace.
// Both the sparse ({ns}_objects) and dense ({ns}_objects_dense) collections are cleaned up.
// The id_mappings entry is retained so the numeric point ID remains stable if the object
// is re-created later.
//
// Caveat: recommendation results cached in Redis may still include this object for up to
// recCacheTTL (5 minutes) after deletion, since the cache is keyed by subject rather than
// by individual objects.
func (s *Service) DeleteObject(ctx context.Context, namespace, objectID string) error {
	numID, err := s.idmapSvc.GetOrCreateObjectID(ctx, objectID, namespace)
	if err != nil {
		return fmt.Errorf("get numeric id: %w", err)
	}

	pointIDs := []*qdrant.PointId{qdrant.NewIDNum(numID)}

	if err := s.deleteFromCollectionFn(ctx, namespace+"_objects", pointIDs); err != nil {
		return err
	}

	// Dense collection is optional — it may not exist when dense_strategy is "disabled"
	// or no embeddings have been pushed yet. Treat cleanup errors as best-effort.
	if err := s.deleteFromCollectionFn(ctx, namespace+"_objects_dense", pointIDs); err != nil {
		slog.Debug("delete object: dense collection cleanup skipped", "namespace", namespace, "object_id", objectID, "error", err)
	}

	return nil
}

func (s *Service) deleteFromCollection(ctx context.Context, collection string, ids []*qdrant.PointId) error {
	err := s.qdrantDeleteFn(ctx, &qdrant.DeletePoints{
		CollectionName: collection,
		Points: &qdrant.PointsSelector{
			PointsSelectorOneOf: &qdrant.PointsSelector_Points{
				Points: &qdrant.PointsIdsList{
					Ids: ids,
				},
			},
		},
	})
	if err != nil {
		// Treat a missing collection as a successful no-op: the object never had a vector
		// there (e.g. the cron job hasn't run yet), so there is nothing to delete.
		if grpcstatus.Code(err) == codes.NotFound {
			return nil
		}
		return fmt.Errorf("delete from %q: %w", collection, err)
	}
	return nil
}

func recCacheKey(namespace, subjectID string, limit int) string {
	return fmt.Sprintf("rec:%s:%s:limit=%d", namespace, subjectID, limit)
}
