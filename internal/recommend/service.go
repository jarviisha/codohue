package recommend

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/jarviisha/codohue/internal/core/idmap"
	"github.com/jarviisha/codohue/internal/core/namespace"
	"github.com/jarviisha/codohue/internal/core/nslifecycle"
	"github.com/jarviisha/codohue/internal/infra/metrics"
	infraqdrant "github.com/jarviisha/codohue/internal/infra/qdrant"
	infraredis "github.com/jarviisha/codohue/internal/infra/redis"
	"github.com/jarviisha/codohue/pkg/codohuetypes"
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

	// dotNormK is the half-saturation constant of the batch-independent
	// score map x/(x+k) applied to unbounded dot products: a raw dot of k
	// maps to 0.5. It is a single global constant on purpose — a
	// per-namespace or per-batch value would reintroduce the cross-request
	// incomparability the map exists to remove. Since compute started
	// L2-normalizing sparse vectors, sparse scores are cosines and only
	// clamp; the curve remains for dot-distance dense namespaces, whose
	// BYOE vectors arrive with arbitrary magnitude.
	dotNormK = 5.0

	// denseDistanceDot mirrors infra/qdrant's distance vocabulary.
	denseDistanceDot = "dot"
)

// ErrCatalogActive is returned by StoreObjectEmbedding when the namespace
// has catalog auto-embedding enabled, per FR-018 / R8 source-of-truth
// precedence: BYOE writes for object dense vectors are rejected with 409
// because the catalog is the single source of truth in catalog mode.
// Subject embeddings are unaffected.
var ErrCatalogActive = errors.New("recommend: namespace uses catalog auto-embedding; BYOE writes for object dense vectors are not accepted")

// ErrInvalidEmbedding identifies non-finite vector values.
var ErrInvalidEmbedding = errors.New("recommend: invalid embedding")

// ErrInvalidObjectCreatedAt identifies a BYOE object_created_at beyond the
// permitted future skew. Separate from ErrInvalidEmbedding so the handler can
// answer with the documented error code.
var ErrInvalidObjectCreatedAt = errors.New("recommend: invalid object_created_at")

// maxObjectCreatedAtSkew mirrors ingest's maxOccurredAtSkew: one documented
// clock-skew rule for every client-supplied creation timestamp. The domains
// cannot share the constant (peer imports are forbidden), so keep them in step
// by hand. Exactly at the boundary is accepted.
const maxObjectCreatedAtSkew = 5 * time.Minute

// Namespace resolution failures are split so the handler can answer honestly:
// a namespace that does not exist is 404, one whose config could not be read is
// 503 and safe to retry.
var (
	ErrNamespaceNotFound          = errors.New("recommend: namespace not found")
	ErrNamespaceConfigUnavailable = errors.New("recommend: namespace config unavailable")
)

type recommendRepo interface {
	CountInteractions(ctx context.Context, namespace, subjectID string) (int, error)
	GetSeenItems(ctx context.Context, namespace, subjectID string, seenItemsDays int) ([]string, error)
	GetAuthoredObjects(ctx context.Context, namespace, subjectID string) (objectIDs []string, truncated bool, err error)
	GetPopularItems(ctx context.Context, namespace string, limit int) ([]string, error)
}

type recommendNsConfig interface {
	Get(ctx context.Context, namespace string) (*namespace.Config, error)
}

// objectMetadataDeleter removes an object's row from the objects table.
// Satisfied by objects.Service and injected by cmd/api — the import rule
// forbids recommend from reaching into a peer domain directly.
//
// Note the asymmetry with GetAuthoredObjects, which reads the same table
// straight from this package's repository: reads stay there because the
// exclusion query is per-request and its cap is a Qdrant concern, while the
// write goes through the table's owner.
type objectMetadataDeleter interface {
	Delete(ctx context.Context, namespace, objectID string) error
}

type recommendIDMapper interface {
	GetOrCreateSubjectID(ctx context.Context, subjectID, namespace string) (uint64, error)
	GetOrCreateObjectID(ctx context.Context, objectID, namespace string) (uint64, error)
	GetOrCreateObjectIDs(ctx context.Context, objectIDs []string, namespace string) (map[string]uint64, error)
	LookupSubjectID(ctx context.Context, subjectID, namespace string) (uint64, bool, error)
	LookupObjectID(ctx context.Context, objectID, namespace string) (uint64, bool, error)
	LookupObjectIDs(ctx context.Context, objectIDs []string, namespace string) (map[string]uint64, error)
}

// Service serves recommendations via collaborative filtering or fallback to popular items.
type Service struct {
	repo        recommendRepo
	nsConfigSvc recommendNsConfig
	idmapSvc    recommendIDMapper
	lifecycle   interface {
		WithWriter(context.Context, string, func(context.Context, *nslifecycle.NamespaceLifecycle) error) error
	}

	// optional; object metadata cleanup is skipped when nil
	objectMeta objectMetadataDeleter

	// infrastructure collaborators (see store.go); production adapters are
	// wired by NewService, tests fake them via newServiceWithDeps.
	vectors  vectorStore
	cache    recommendationCache
	trending trendingSource
}

// NewService creates a new Service with all required dependencies, wiring the
// production Qdrant and Redis adapters.
func NewService(
	repo *Repository,
	nsConfigSvc recommendNsConfig,
	idmapSvc *idmap.Service,
	qdrantClient *qdrant.Client,
	redisClient *goredis.Client,
) *Service {
	redis := &redisStore{client: redisClient}
	return newServiceWithDeps(repo, nsConfigSvc, idmapSvc,
		&qdrantVectorStore{client: qdrantClient}, redis, redis)
}

// newServiceWithDeps is the explicitly-supported dependency constructor: the
// single seam through which tests substitute the infrastructure collaborators.
func newServiceWithDeps(
	repo recommendRepo,
	nsConfigSvc recommendNsConfig,
	idmapSvc recommendIDMapper,
	vectors vectorStore,
	cache recommendationCache,
	trending trendingSource,
) *Service {
	return &Service{
		repo:        repo,
		nsConfigSvc: nsConfigSvc,
		idmapSvc:    idmapSvc,
		vectors:     vectors,
		cache:       cache,
		trending:    trending,
	}
}

// SetObjectMetadataDeleter wires the objects domain in so DeleteObject can
// drop the object's metadata row alongside its vectors. The wiring layer
// calls this once at startup.
func (s *Service) SetObjectMetadataDeleter(d objectMetadataDeleter) { s.objectMeta = d }

// SetLifecycleWriter fences BYOE and delete mutations.
func (s *Service) SetLifecycleWriter(writer interface {
	WithWriter(context.Context, string, func(context.Context, *nslifecycle.NamespaceLifecycle) error) error
}) {
	s.lifecycle = writer
}

// StoreObjectEmbedding stores a BYOE dense vector for an object in
// {ns}_objects_dense. createdAt (optional) lands in the point payload so the
// γ-freshness rerank decays BYOE items like sparse-path ones.
func (s *Service) StoreObjectEmbedding(ctx context.Context, ns, objectID string, vector []float32, createdAt *time.Time) error {
	return s.storeEmbedding(ctx, ns, objectID, "object", vector, createdAt)
}

// StoreSubjectEmbedding stores a BYOE dense vector for a subject in {ns}_subjects_dense.
func (s *Service) StoreSubjectEmbedding(ctx context.Context, ns, subjectID string, vector []float32) error {
	return s.storeEmbedding(ctx, ns, subjectID, "subject", vector, nil)
}

func (s *Service) storeEmbedding(ctx context.Context, ns, entityID, entityType string, vector []float32, createdAt *time.Time) error {
	for _, value := range vector {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return fmt.Errorf("%w: vector contains non-finite values", ErrInvalidEmbedding)
		}
	}
	// A future creation time makes the γ-freshness age negative, which boosts
	// the item instead of decaying it. Scoring clamps as a backstop, but the
	// value is rejected here so the stored payload is not quietly wrong.
	if createdAt != nil && createdAt.After(time.Now().UTC().Add(maxObjectCreatedAtSkew)) {
		return fmt.Errorf("%w: object_created_at is more than five minutes in the future", ErrInvalidObjectCreatedAt)
	}
	if s.lifecycle != nil {
		return s.lifecycle.WithWriter(ctx, ns, func(leased context.Context, _ *nslifecycle.NamespaceLifecycle) error {
			return s.storeEmbeddingActive(leased, ns, entityID, entityType, vector, createdAt)
		})
	}
	return s.storeEmbeddingActive(ctx, ns, entityID, entityType, vector, createdAt)
}

func (s *Service) storeEmbeddingActive(ctx context.Context, ns, entityID, entityType string, vector []float32, createdAt *time.Time) error {
	cfg, err := s.nsConfigSvc.Get(ctx, ns)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNamespaceConfigUnavailable, err)
	}
	if cfg == nil {
		return ErrNamespaceNotFound
	}

	// FR-018 / R8 source-of-truth precedence: when dense_source is "catalog",
	// BYOE writes for OBJECT dense vectors are rejected so the catalog stays
	// the single source of truth. Subject embeddings are NOT guarded — subject
	// vectors keep flowing through the cron mean-pool / BYOE path regardless.
	if cfg != nil && cfg.DenseSource == codohuetypes.DenseSourceCatalog && entityType == "object" {
		return ErrCatalogActive
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
	// This is a write path: the lease is the authority on which incarnation
	// to address; config covers the lifecycle-less (test) construction.
	inc, ok := nslifecycle.LeaseIncarnation(ctx, ns)
	if !ok {
		inc = nslifecycle.ConfigIncarnation(ns, cfg)
	}
	if err := s.vectors.EnsureDenseCollections(ctx, inc, dim, distance); err != nil {
		return fmt.Errorf("ensure dense collections: %w", err)
	}

	// Resolve collection name.
	kind := infraqdrant.CollectionSubjectsDense
	if entityType == "object" {
		kind = infraqdrant.CollectionObjectsDense
	}
	collection := infraqdrant.CollectionName(inc, kind)
	idKey := entityType + "_id"

	// Get or create numeric ID.
	var numID uint64
	if entityType == "object" {
		numID, err = s.idmapSvc.GetOrCreateObjectID(ctx, entityID, ns)
	} else {
		numID, err = s.idmapSvc.GetOrCreateSubjectID(ctx, entityID, ns)
	}
	if err != nil {
		return fmt.Errorf("get numeric id: %w", err)
	}

	payload := map[string]*qdrant.Value{
		idKey:        qdrant.NewValueString(entityID),
		"strategy":   qdrant.NewValueString("byoe"),
		"updated_at": qdrant.NewValueString(time.Now().UTC().Format(time.RFC3339)),
	}
	if createdAt != nil {
		payload["created_at"] = qdrant.NewValueString(createdAt.UTC().Format(time.RFC3339))
	}

	if err := s.vectors.UpsertDensePoint(ctx, collection, numID, vector, payload); err != nil {
		return fmt.Errorf("upsert dense vector: %w", err)
	}
	return nil
}

// Recommend returns recommended items for a subject, selecting the strategy based on interaction history.
func (s *Service) Recommend(ctx context.Context, req *Request) (*Response, error) {
	if req.Limit <= 0 {
		req.Limit = 20
	}
	if req.Offset < 0 {
		req.Offset = 0
	}

	cfg, err := s.nsConfigSvc.Get(ctx, req.Namespace)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNamespaceConfigUnavailable, err)
	}
	if cfg == nil {
		return nil, ErrNamespaceNotFound
	}
	maxResults := req.Limit
	if cfg != nil && cfg.MaxResults > 0 && cfg.MaxResults < maxResults {
		maxResults = cfg.MaxResults
	}

	cacheKey := recCacheKey(req.Namespace, namespaceGeneration(cfg), req.SubjectID, maxResults, req.Offset)
	if cached, err := s.cache.Get(ctx, cacheKey); err == nil {
		var resp Response
		if json.Unmarshal([]byte(cached), &resp) == nil &&
			resp.Namespace == req.Namespace && resp.SubjectID == req.SubjectID {
			metrics.RedisCacheRequests.WithLabelValues("hit").Inc()
			return &resp, nil
		}
	}
	metrics.RedisCacheRequests.WithLabelValues("miss").Inc()

	out, err := s.doRecommend(ctx, req, maxResults, cfg)
	if err != nil {
		return nil, err
	}
	resp := &Response{
		SubjectID:   req.SubjectID,
		Namespace:   req.Namespace,
		Items:       out.items,
		Source:      out.source,
		Limit:       maxResults,
		Offset:      req.Offset,
		Total:       out.total,
		GeneratedAt: time.Now().UTC(),
	}

	// The cache decision is made exactly once, here, from the outcome: a
	// degraded outcome (fallback caused by an infra error) is served but
	// never cached, so a one-second Qdrant blip does not pin popular items
	// on a warm subject for the full TTL.
	if !out.degraded {
		if b, err := json.Marshal(resp); err == nil {
			s.cache.Set(ctx, cacheKey, string(b), recCacheTTL)
		}
	}
	return resp, nil
}

// scoreScale documents, internally, what an outcome's Score values mean. It
// never reaches the wire: the JSON bytes clients see are unchanged.
type scoreScale int

const (
	// scaleUnscored is the fallback scale: items carry the literal 0 with
	// Scored=false because no per-subject relevance verdict exists.
	scaleUnscored scoreScale = iota
	// scaleRawFreshness is CF's scale: the raw Qdrant dot product (a cosine
	// since compute L2-normalizes sparse vectors) times γ-freshness decay,
	// not bounded to [0, 1] by construction.
	scaleRawFreshness
	// scaleUnitBlend is the hybrid scale: the α-blend of unit-normalized
	// sparse and dense arms times γ-freshness decay, bounded to [0, 1].
	scaleUnitBlend
)

// outcome is what one recommendation-ladder rung produces: the served page
// plus the facts Recommend needs to build the wire Response and decide
// cacheability. Degradation flows back through this value — never through
// mutation of the caller's Request.
type outcome struct {
	items  []RecommendedItem
	source string
	total  int
	scale  scoreScale
	// degraded records that this outcome is a fallback caused by an
	// infrastructure error (Qdrant/Redis/DB unavailable) rather than by the
	// subject's data state (cold start, empty index). Recommend never caches
	// a degraded outcome.
	degraded bool
}

// doRecommend is the recommendation ladder; the full rung order is stated
// here and nowhere else:
//
//	count == 0  → fallbackTrending
//	              (descends to fallbackPopular when Redis fails — degraded —
//	               or when the namespace has no trending data at all)
//	count < coldStartThreshold
//	            → hybridCold, which blends collaborativeFiltering with
//	              fallbackTrending and serves the surviving share when one
//	              side fails (degraded)
//	otherwise   → collaborativeFiltering
//	              (hands off to hybridRecommend when hybridEligible and the
//	               subject has a dense vector; any infra failure descends to
//	               fallbackPopular via descendPopular — degraded)
//
// Each rung returns an outcome, never an HTTP-shaped Response; Recommend
// assembles the wire shape and makes the cache decision from the outcome.
func (s *Service) doRecommend(ctx context.Context, req *Request, maxResults int, cfg *namespace.Config) (outcome, error) {
	count, err := s.repo.CountInteractions(ctx, req.Namespace, req.SubjectID)
	// A count failure sends a possibly warm subject down the cold-start
	// rung — degraded, whatever that rung then serves.
	degraded := err != nil
	if err != nil {
		slog.Error("count interactions failed", "namespace", req.Namespace, "subject_id", req.SubjectID, "error", err)
	}

	var out outcome
	switch {
	case count == 0:
		out, err = s.fallbackTrending(ctx, req, maxResults, cfg, nil)
	case count < coldStartThreshold:
		out, err = s.hybridCold(ctx, req, maxResults, cfg)
	default:
		out, err = s.collaborativeFiltering(ctx, req, maxResults, cfg)
	}
	if err != nil {
		return outcome{}, err
	}
	out.degraded = out.degraded || degraded
	return out, nil
}

// descendPopular is the ladder's one downward edge: every rung that cannot
// serve falls to fallbackPopular through this call, stating explicitly
// whether the descent was caused by infrastructure (degraded — the outcome
// must not be cached) or by data state (cacheable).
func (s *Service) descendPopular(ctx context.Context, req *Request, limit int, cfg *namespace.Config, exclude map[string]struct{}, degraded bool) (outcome, error) {
	out, err := s.fallbackPopular(ctx, req, limit, cfg, exclude)
	if err != nil {
		return outcome{}, err
	}
	out.degraded = out.degraded || degraded
	return out, nil
}

// hybridEligible is the single hybrid eligibility gate shared by Recommend's
// CF rung and Rank: the namespace blend must leave dense weight (0 < α < 1)
// and a dense source must be configured. internal/admin's dashboard carries a
// display-only variant on its own summary type; that is out of scope here.
func hybridEligible(cfg *namespace.Config) bool {
	return cfg != nil && cfg.Alpha > 0 && cfg.Alpha < 1.0 &&
		cfg.DenseSource != "" && cfg.DenseSource != codohuetypes.DenseSourceDisabled
}

func (s *Service) collaborativeFiltering(ctx context.Context, req *Request, limit int, cfg *namespace.Config) (outcome, error) {
	subjectNumID, found, err := s.idmapSvc.LookupSubjectID(ctx, req.SubjectID, req.Namespace)
	if err != nil || !found {
		slog.Error("get subject numeric id failed", "namespace", req.Namespace, "subject_id", req.SubjectID, "error", err)
		return s.descendPopular(ctx, req, limit, cfg, nil, true)
	}

	physicalNamespace := qdrantPhysicalNamespace(req.Namespace, cfg)
	subjectVec, err := s.vectors.FetchSubjectVector(ctx, physicalNamespace, subjectNumID)
	if err != nil || subjectVec == nil {
		slog.Error("fetch subject vector failed", "namespace", req.Namespace, "subject_id", req.SubjectID, "error", err)
		// err != nil is an infra failure; a nil vector without error just
		// means the cron batch has not caught up with this subject yet —
		// that state is stable for the tick, so caching it is fine.
		return s.descendPopular(ctx, req, limit, cfg, nil, err != nil)
	}

	seenItemsDays := 30
	if cfg != nil && cfg.SeenItemsDays > 0 {
		seenItemsDays = cfg.SeenItemsDays
	}
	seenItems, err := s.repo.GetSeenItems(ctx, req.Namespace, req.SubjectID, seenItemsDays)
	if err != nil {
		slog.Error("get seen items failed", "namespace", req.Namespace, "subject_id", req.SubjectID, "error", err)
	}
	seenFilter := s.buildSeenItemsFilter(ctx, req.Namespace,
		s.excludedObjectIDs(ctx, req, cfg, seenItems))

	// Use hybrid scoring when alpha < 1.0 and dense strategy is active.
	// Note: subject dense vectors are computed during the cron batch run and may be up
	// to one cron interval stale. New interactions since the last batch are not reflected
	// in the dense component. The sparse CF component is unaffected — it queries Qdrant
	// against vectors recomputed in the same batch. To reduce staleness, decrease
	// CODOHUE_BATCH_INTERVAL_MINUTES or push subject embeddings via BYOE after each interaction.
	if hybridEligible(cfg) {
		denseVec, err := s.vectors.FetchSubjectDenseVector(ctx, physicalNamespace, subjectNumID)
		if err == nil && denseVec != nil {
			return s.hybridRecommend(ctx, req, limit, cfg, subjectVec, denseVec, seenFilter)
		}
		// Subject has no dense vector yet — fall through to pure sparse CF.
		// Warn, not Debug: a namespace-wide empty {ns}_subjects_dense means
		// the config says hybrid while every request silently serves
		// sparse-only (the standing failure mode of BYOE namespaces that
		// never push subject vectors). The admin overview raises the
		// fleet-level alert; this is the per-request trace.
		slog.Warn("hybrid: no subject dense vector, serving pure sparse CF despite dense config",
			"namespace", req.Namespace, "subject_id", req.SubjectID, "dense_source", cfg.DenseSource, "error", err)
	}

	// Over-fetch enough to cover offset + limit after reranking.
	fetchLimit := uint64((req.Offset + limit) * cfOverFetchFactor)
	results, err := s.vectors.SearchObjects(ctx, physicalNamespace, subjectVec, seenFilter, fetchLimit)
	if err != nil {
		slog.Error("search objects failed", "namespace", req.Namespace, "subject_id", req.SubjectID, "error", err)
		return s.descendPopular(ctx, req, limit, cfg, nil, true)
	}

	scored := rerankScored(results, resolveGamma(cfg), req.Offset+limit)

	metrics.RecommendRequests.WithLabelValues(req.Namespace, SourceCollaborativeFiltering).Inc()
	return outcome{
		items:  pageItems(scored, req.Offset, limit),
		source: SourceCollaborativeFiltering,
		total:  len(scored),
		scale:  scaleRawFreshness,
	}, nil
}

// hybridRecommend performs hybrid retrieval (sparse + dense) and blends scores.
func (s *Service) hybridRecommend(
	ctx context.Context,
	req *Request,
	limit int,
	cfg *namespace.Config,
	subjectSparseVec *qdrant.SparseVector,
	subjectDenseVec []float32,
	seenFilter *qdrant.Filter,
) (outcome, error) {
	alpha := cfg.Alpha
	physicalNamespace := qdrantPhysicalNamespace(req.Namespace, cfg)
	degraded := false

	// Over-fetch enough to cover offset + limit.
	sparseTopK := uint64((req.Offset + limit) * cfOverFetchFactor)
	denseTopK := uint64((req.Offset + limit) * denseOverFetchFactor)

	// Sparse retrieval.
	sparseOK := true
	sparseResults, err := s.vectors.SearchObjects(ctx, physicalNamespace, subjectSparseVec, seenFilter, sparseTopK)
	if err != nil {
		slog.Error("hybrid: sparse search failed", "namespace", req.Namespace, "error", err)
		sparseResults = nil
		sparseOK = false
		degraded = true
	}

	// Dense retrieval.
	denseOK := true
	denseResults, err := s.vectors.SearchObjectsDense(ctx, physicalNamespace, subjectDenseVec, seenFilter, denseTopK)
	if err != nil {
		slog.Error("hybrid: dense search failed", "namespace", req.Namespace, "error", err)
		denseResults = nil
		denseOK = false
		degraded = true
	}

	if len(sparseResults) == 0 && len(denseResults) == 0 {
		return s.descendPopular(ctx, req, limit, cfg, nil, degraded)
	}

	// Each arm's top-K must also be scored by the other arm before blending.
	// The blend treats a missing score as 0, but for a candidate that simply
	// fell outside the other arm's top-K, 0 means "not retrieved", not
	// "dissimilar" — under that reading a dense-only candidate capped at
	// (1-alpha) and could never outrank an ordinary sparse one. Rank solved
	// the same problem by scoring both arms over one HasID candidate set;
	// this borrows the mechanism for the retrieved union. After the
	// backfill a still-missing score is a real verdict: the object is not
	// indexed on that side (e.g. no catalog embedding), and 0 is honest.
	//
	// An arm whose primary search just failed is not backfilled: re-querying
	// a backend that errored milliseconds ago adds load exactly when the
	// service should shed it, and a retry that happens to succeed yields
	// neither the healthy blend nor the documented zero-fill degradation.
	//
	// Backfill failure degrades to the zero-fill reading rather than failing
	// the request, but it marks the response degraded all the same: the
	// suppressed ordering is as unfit to cache for the full TTL as the one a
	// failed primary search produces.
	// Both id sets are read before either search runs, so the two backfills
	// are order-independent and can go out concurrently: one added round-trip
	// instead of two.
	var sparseMissing, denseMissing []*qdrant.PointId
	if sparseOK {
		sparseMissing = pointIDsMissingFrom(sparseResults, denseResults)
	}
	if denseOK {
		denseMissing = pointIDsMissingFrom(denseResults, sparseResults)
	}

	var wg sync.WaitGroup
	var sparseExtra, denseExtra []*qdrant.ScoredPoint
	var sparseErr, denseErr error
	if len(sparseMissing) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sparseExtra, sparseErr = s.vectors.SearchObjects(ctx, physicalNamespace, subjectSparseVec,
				hasIDFilter(sparseMissing), uint64(len(sparseMissing)))
		}()
	}
	if len(denseMissing) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			denseExtra, denseErr = s.vectors.SearchObjectsDense(ctx, physicalNamespace, subjectDenseVec,
				hasIDFilter(denseMissing), uint64(len(denseMissing)))
		}()
	}
	wg.Wait()

	if sparseErr != nil {
		slog.Warn("hybrid: sparse backfill failed; dense-only candidates keep sparse 0", "namespace", req.Namespace, "error", sparseErr)
		degraded = true
	} else {
		sparseResults = append(sparseResults, scoresOnly(sparseExtra)...)
	}
	if denseErr != nil {
		slog.Warn("hybrid: dense backfill failed; sparse-only candidates keep dense 0", "namespace", req.Namespace, "error", denseErr)
		degraded = true
	} else {
		denseResults = append(denseResults, scoresOnly(denseExtra)...)
	}

	// Effective alpha, matching Rank: the namespace blend when both arms
	// answered, full weight to the surviving arm otherwise. Scaling a lone
	// side by alpha would shrink every score for no reason, and would make
	// the same outage return differently-scaled scores from the two endpoints
	// that share this blend.
	switch {
	case sparseOK && denseOK:
	case denseOK:
		alpha = 0.0
	default:
		alpha = 1.0
	}

	candidates := blendHybridScores(sparseResults, denseResults, alpha, resolveGamma(cfg), cfg.DenseDistance, time.Now().UTC())

	paged, start := pageOf(candidates, req.Offset, limit)
	items := make([]RecommendedItem, len(paged))
	for i, c := range paged {
		items[i] = RecommendedItem{
			ObjectID: c.objectID,
			Score:    c.score,
			Rank:     start + i + 1,
			Scored:   true,
		}
	}

	metrics.RecommendRequests.WithLabelValues(req.Namespace, SourceHybrid).Inc()
	return outcome{
		items:    items,
		source:   SourceHybrid,
		total:    len(candidates),
		scale:    scaleUnitBlend,
		degraded: degraded,
	}, nil
}

// extractScores builds an objectID → raw score map from Qdrant results.
func extractScores(points []*qdrant.ScoredPoint) map[string]float64 {
	m := make(map[string]float64, len(points))
	for _, p := range points {
		if id, ok := objectIDOf(p); ok {
			m[id] = float64(p.Score)
		}
	}
	return m
}

// clampUnitScores puts scores that are cosines by construction on a fixed
// [0, 1] scale: negatives clamp to 0 (dissimilar must not drag the blend
// below "no signal") and float rounding artifacts above 1 clamp to 1. Sparse
// scores qualify because compute L2-normalizes both sides of the sparse dot
// product; the previous saturating curve x/(x+5) was calibrated for raw dots
// in the low single digits and, against live data whose raw dots reached the
// thousands, flattened the entire top of the ranking into ~1.0 — freshness
// decay, not relevance, was deciding the order. Like the curve it replaces,
// the mapping does not depend on the other scores in the request, so scores
// from separate calls stay comparable — chunked Rank callers merge results
// from multiple requests into one ordering.
func clampUnitScores(scores map[string]float64) map[string]float64 {
	result := make(map[string]float64, len(scores))
	for id, v := range scores {
		if !finiteScore(v) {
			continue
		}
		result[id] = clampUnit(v)
	}
	return result
}

// boundDenseScores puts dense similarities on a fixed [0, 1] scale. Cosine
// scores are bounded and clamp like sparse. Dot-product namespaces carry
// BYOE vectors of arbitrary magnitude, so they go through the saturating
// curve x/(x+dotNormK) instead.
func boundDenseScores(scores map[string]float64, distance string) map[string]float64 {
	if distance != denseDistanceDot {
		return clampUnitScores(scores)
	}
	result := make(map[string]float64, len(scores))
	for id, v := range scores {
		if !finiteScore(v) {
			continue
		}
		if v <= 0 {
			result[id] = 0
			continue
		}
		result[id] = v / (v + dotNormK)
	}
	return result
}

// pointIDsMissingFrom returns the Qdrant point ids of candidates present in
// have but absent from base, keyed by the object_id payload both arms carry.
// Feeding these to a HasID search scores them on base's arm.
func pointIDsMissingFrom(base, have []*qdrant.ScoredPoint) []*qdrant.PointId {
	inBase := make(map[string]struct{}, len(base))
	for _, p := range base {
		if id, ok := objectIDOf(p); ok {
			inBase[id] = struct{}{}
		}
	}
	ids := make([]*qdrant.PointId, 0, len(have))
	for _, p := range have {
		id, ok := objectIDOf(p)
		if !ok {
			continue
		}
		if _, seen := inBase[id]; !seen {
			ids = append(ids, p.Id)
		}
	}
	return ids
}

// hasIDFilter restricts a search to an explicit point-id set — the mechanism
// Rank uses to score a caller's candidates, borrowed here for the backfill.
func hasIDFilter(ids []*qdrant.PointId) *qdrant.Filter {
	return &qdrant.Filter{Must: []*qdrant.Condition{qdrant.NewHasID(ids...)}}
}

// objectIDOf reads the object_id a scored point carries. One definition of
// "which object is this point" keeps the backfill's membership test and the
// blend's scoring keyed the same way: if they diverged, the backfill would
// fetch candidates the blend then ignores.
func objectIDOf(p *qdrant.ScoredPoint) (string, bool) {
	v, ok := p.Payload["object_id"]
	if !ok {
		return "", false
	}
	return v.GetStringValue(), true
}

// scoresOnly drops created_at from backfilled points. A backfilled point
// exists only to carry its arm's score: its object_id is by construction
// already present in the arm that asked for the backfill, and that arm's
// payload owns the object's creation time. Left in place, the backfilled
// copy could win buildCreatedAtLookup's first-seen-wins pass and replace a
// true creation time with cron's fallback — the last interaction — which
// would hand a two-year-old object the freshness multiplier of a new one.
func scoresOnly(points []*qdrant.ScoredPoint) []*qdrant.ScoredPoint {
	for _, p := range points {
		delete(p.Payload, "created_at")
	}
	return points
}

// blendedCandidate is one object scored by the shared hybrid blend.
type blendedCandidate struct {
	objectID string
	score    float64
}

// blendHybridScores is the single blend definition shared by Recommend and
// Rank: normalize each side batch-independently, α-blend, apply γ freshness
// decay, sort descending with object id as the deterministic tie-break.
// Callers with only one side available pass alpha 1 (sparse-only) or 0
// (dense-only) so the present side keeps full weight.
func blendHybridScores(sparseResults, denseResults []*qdrant.ScoredPoint, alpha, gamma float64, denseDistance string, now time.Time) []blendedCandidate {
	normSparse := clampUnitScores(extractScores(sparseResults))
	normDense := boundDenseScores(extractScores(denseResults), denseDistance)

	candidateSet := make(map[string]struct{}, len(normSparse)+len(normDense))
	for id := range normSparse {
		candidateSet[id] = struct{}{}
	}
	for id := range normDense {
		candidateSet[id] = struct{}{}
	}

	createdAt := buildCreatedAtLookup(sparseResults, denseResults)

	candidates := make([]blendedCandidate, 0, len(candidateSet))
	for objectID := range candidateSet {
		blended := alpha*normSparse[objectID] + (1-alpha)*normDense[objectID]
		if t, ok := createdAt[objectID]; ok {
			blended *= freshnessMultiplier(now, t, gamma)
		}
		if !finiteScore(blended) {
			continue
		}
		candidates = append(candidates, blendedCandidate{objectID: objectID, score: blended})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].objectID < candidates[j].objectID
	})
	return candidates
}

// resolveGamma returns the namespace's freshness decay or the default.
func resolveGamma(cfg *namespace.Config) float64 {
	if cfg != nil && cfg.Gamma > 0 {
		return cfg.Gamma
	}
	return defaultGamma
}

// buildCreatedAtLookup extracts created_at timestamps from Qdrant payloads.
func buildCreatedAtLookup(sets ...[]*qdrant.ScoredPoint) map[string]time.Time {
	m := make(map[string]time.Time)
	for _, pts := range sets {
		for _, p := range pts {
			id, ok := objectIDOf(p)
			if !ok {
				continue
			}
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

func (s *Service) hybridCold(ctx context.Context, req *Request, limit int, cfg *namespace.Config) (outcome, error) {
	// Over-fetch from both sources to cover offset before blending. The
	// inner requests page from 0, so every branch below must re-apply the
	// caller's offset before returning — including the degraded ones.
	overLimit := req.Offset + limit
	innerReq := &Request{
		SubjectID: req.SubjectID,
		Namespace: req.Namespace,
		Limit:     overLimit,
		Offset:    0,
	}

	// The CF sub-call filters seen items via the Qdrant MustNot; the
	// trending share must drop them too, or the item the subject touched
	// five minutes ago (likely trending) comes straight back in 70% of
	// their recommendations.
	seenItemsDays := 30
	if cfg != nil && cfg.SeenItemsDays > 0 {
		seenItemsDays = cfg.SeenItemsDays
	}
	var seen map[string]struct{}
	if seenItems, err := s.repo.GetSeenItems(ctx, req.Namespace, req.SubjectID, seenItemsDays); err != nil {
		slog.Error("hybrid cold: get seen items failed", "namespace", req.Namespace, "subject_id", req.SubjectID, "error", err)
	} else if len(seenItems) > 0 {
		seen = make(map[string]struct{}, len(seenItems))
		for _, id := range seenItems {
			seen[id] = struct{}{}
		}
	}

	cfOut, cfErr := s.collaborativeFiltering(ctx, innerReq, overLimit, cfg)
	popOut, popErr := s.fallbackTrending(ctx, innerReq, overLimit, cfg, seen)
	// Degradation flows back through the sub-outcomes — no hand-propagation
	// across a shared request struct.
	degraded := cfOut.degraded || popOut.degraded

	if popErr != nil && cfErr != nil {
		return outcome{}, fmt.Errorf("hybrid cold: popular: %w; cf: %v", popErr, cfErr)
	}
	if popErr != nil {
		// Serve the CF share alone, relabeled: its items keep their real
		// scores and inner scale (see the pass-through test pinning this).
		cfOut.source = SourceHybridCold
		cfOut.items = repageItems(cfOut.items, req.Offset, limit)
		cfOut.degraded = true
		return cfOut, nil
	}
	if cfErr != nil || len(cfOut.items) == 0 {
		popOut.items = repageItems(popOut.items, req.Offset, limit)
		popOut.degraded = degraded || cfErr != nil
		return popOut, nil
	}

	blended := blendItems(itemIDs(popOut.items), itemIDs(cfOut.items), 0.7, overLimit)
	paged, start := pageOf(blended, req.Offset, limit)

	items := make([]RecommendedItem, len(paged))
	for i, id := range paged {
		// blendItems interleaves two differently-scaled lists by position, so
		// the CF score that survived into the blend no longer describes the
		// item's rank here. Report the ordering without a score rather than a
		// 0 that reads as "irrelevant".
		items[i] = RecommendedItem{ObjectID: id, Score: 0, Rank: start + i + 1, Scored: false}
	}

	metrics.RecommendRequests.WithLabelValues(req.Namespace, SourceHybridCold).Inc()
	return outcome{
		items:    items,
		source:   SourceHybridCold,
		total:    len(blended),
		scale:    scaleUnscored,
		degraded: degraded,
	}, nil
}

// GetTrending returns the trending items for a namespace from Redis.
// There is exactly one trending ZSET per namespace, computed by cron with
// the namespace's configured window — the response reports that actual
// window rather than pretending a per-request override exists.
func (s *Service) GetTrending(ctx context.Context, ns string, limit, offset int) (*TrendingResponse, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	cfg, err := s.nsConfigSvc.Get(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNamespaceConfigUnavailable, err)
	}
	if cfg == nil {
		return nil, ErrNamespaceNotFound
	}

	actualWindow := 24
	if cfg != nil && cfg.TrendingWindow > 0 {
		actualWindow = cfg.TrendingWindow
	}

	entries, err := s.trending.GetTrending(ctx, ns, namespaceGeneration(cfg), offset, limit)
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
		Limit:       limit,
		Offset:      offset,
		Total:       len(items),
		GeneratedAt: time.Now().UTC(),
	}, nil
}

// fallbackTrending serves trending items from Redis as the cold-start response.
// The DB-popular fallback is used only when there is NO trending data at all
// (or Redis is unavailable) — a merely empty page means the client paginated
// past the end of trending and must get an empty page back, not a switch to a
// differently-ordered list whose items were already served. exclude carries
// extra object ids to drop (hybridCold passes the subject's seen items);
// nil is the common case.
func (s *Service) fallbackTrending(ctx context.Context, req *Request, limit int, cfg *namespace.Config, exclude map[string]struct{}) (outcome, error) {
	// Exclusions (authored and seen) have to happen before paging or the
	// offset would count rows that are about to be dropped. Fetch from
	// rank 0 with enough headroom to survive removing every excluded object.
	excluded := mergeExclusions(s.authoredObjectSet(ctx, req, cfg), exclude)
	fetchOffset, fetchLimit := req.Offset, limit
	if len(excluded) > 0 {
		fetchOffset, fetchLimit = 0, req.Offset+limit+len(excluded)
	}

	generation := namespaceGeneration(cfg)
	entries, err := s.trending.GetTrending(ctx, req.Namespace, generation, fetchOffset, fetchLimit)
	if err != nil {
		slog.Error("get trending failed, serving popular", "namespace", req.Namespace, "error", err)
		return s.descendPopular(ctx, req, limit, cfg, exclude, true)
	}

	hasTrending := len(entries) > 0
	if !hasTrending && fetchOffset > 0 {
		// Empty page at a non-zero offset: distinguish "past the end of
		// trending" from "no trending data" by probing rank 0.
		if probe, probeErr := s.trending.GetTrending(ctx, req.Namespace, generation, 0, 1); probeErr == nil && len(probe) > 0 {
			hasTrending = true
		}
	}
	if !hasTrending {
		// No trending data at all is a data state, not degradation.
		return s.descendPopular(ctx, req, limit, cfg, exclude, false)
	}

	if len(excluded) > 0 {
		entries = dropAuthoredEntries(entries, excluded)
		entries, _ = pageOf(entries, req.Offset, limit)
	}

	// entries may be empty here — that is the page that terminates the
	// client's pagination.
	items := make([]RecommendedItem, len(entries))
	for i, e := range entries {
		// The trending ZSET score is a namespace-wide popularity count, not a
		// score for this subject; TrendingResponse exposes it where it means
		// something.
		items[i] = RecommendedItem{
			ObjectID: e.ObjectID,
			Score:    0,
			Rank:     req.Offset + i + 1,
			Scored:   false,
		}
	}
	metrics.RecommendRequests.WithLabelValues(req.Namespace, SourceFallbackPopular).Inc()
	return outcome{
		items:  items,
		source: SourceFallbackPopular,
		total:  req.Offset + len(items),
		scale:  scaleUnscored,
	}, nil
}

func (s *Service) fallbackPopular(ctx context.Context, req *Request, limit int, cfg *namespace.Config, exclude map[string]struct{}) (outcome, error) {
	// Fetch enough rows to cover offset + limit so we can slice in-process,
	// plus headroom for the excluded objects about to be dropped — filtering
	// has to happen before paging or the offset would count removed rows.
	excluded := mergeExclusions(s.authoredObjectSet(ctx, req, cfg), exclude)
	rawItems, err := s.repo.GetPopularItems(ctx, req.Namespace, req.Offset+limit+len(excluded))
	if err != nil {
		return outcome{}, fmt.Errorf("get popular items: %w", err)
	}
	rawItems = dropAuthored(rawItems, excluded)
	total := len(rawItems)

	paged, start := pageOf(rawItems, req.Offset, limit)
	items := make([]RecommendedItem, len(paged))
	for i, id := range paged {
		items[i] = RecommendedItem{ObjectID: id, Score: 0, Rank: start + i + 1, Scored: false}
	}

	metrics.RecommendRequests.WithLabelValues(req.Namespace, SourceFallbackPopular).Inc()
	return outcome{
		items:  items,
		source: SourceFallbackPopular,
		total:  total,
		scale:  scaleUnscored,
	}, nil
}

// buildSeenItemsFilter builds the MustNot point-id filter applied to every
// object search. Both exclusion reasons — already seen, and (when
// exclude_authored is on) authored by the requester — collapse into a single
// HasID condition, which is what lets the same filter cover the sparse
// ({ns}_objects) and dense ({ns}_objects_dense) collections alike. Filtering
// on a payload field instead would only work for the dense collection, since
// cmd/cron writes the sparse points and knows nothing about authorship.
func (s *Service) buildSeenItemsFilter(ctx context.Context, ns string, excludedStringIDs []string) *qdrant.Filter {
	if len(excludedStringIDs) == 0 {
		return nil
	}
	// One round-trip for the whole exclusion set — at the 5000-id authored
	// cap the per-id variant was ~5000 sequential queries per uncached
	// request. Errors degrade to an unfiltered search, same as before.
	numIDs, err := s.idmapSvc.LookupObjectIDs(ctx, excludedStringIDs, ns)
	if err != nil {
		slog.Error("build seen filter: batch id mapping failed, serving unfiltered", "namespace", ns, "error", err)
		return nil
	}
	ids := make([]*qdrant.PointId, 0, len(numIDs))
	for _, numID := range numIDs {
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

// authoredObjectSet returns the objects the subject authored, as a set, or
// nil when the namespace has the filter switched off. A nil return means
// "apply no author filtering" and is the normal case.
//
// Query errors degrade to nil rather than failing the request: serving a
// recommendation that includes the subject's own objects beats serving none.
func (s *Service) authoredObjectSet(ctx context.Context, req *Request, cfg *namespace.Config) map[string]struct{} {
	if cfg == nil || !cfg.ExcludeAuthored {
		return nil
	}

	authored, truncated, err := s.repo.GetAuthoredObjects(ctx, req.Namespace, req.SubjectID)
	if err != nil {
		slog.Error("exclude authored: query failed, serving unfiltered",
			"namespace", req.Namespace, "subject_id", req.SubjectID, "error", err)
		return nil
	}
	if truncated {
		slog.Warn("exclude authored: hit the cap, older authored objects may still be recommended",
			"namespace", req.Namespace, "subject_id", req.SubjectID, "cap", defaultAuthoredObjectsCap)
	}
	if len(authored) == 0 {
		return nil
	}

	set := make(map[string]struct{}, len(authored))
	for _, id := range authored {
		set[id] = struct{}{}
	}
	return set
}

// excludedObjectIDs merges the seen-items list with the subject's own authored
// objects. De-duplicated because an author who interacted with their own
// object appears in both lists and would otherwise be sent to Qdrant twice.
func (s *Service) excludedObjectIDs(ctx context.Context, req *Request, cfg *namespace.Config, seenItems []string) []string {
	authored := s.authoredObjectSet(ctx, req, cfg)
	if len(authored) == 0 {
		return seenItems
	}

	merged := make([]string, 0, len(seenItems)+len(authored))
	dup := make(map[string]struct{}, len(seenItems)+len(authored))
	for _, id := range seenItems {
		if _, ok := dup[id]; ok {
			continue
		}
		dup[id] = struct{}{}
		merged = append(merged, id)
	}
	for id := range authored {
		if _, ok := dup[id]; ok {
			continue
		}
		dup[id] = struct{}{}
		merged = append(merged, id)
	}
	return merged
}

// dropAuthored removes authored ids from an ordered candidate list. Used by
// the trending / popular fallbacks, which cannot push the exclusion down into
// the store the way the Qdrant paths can.
func dropAuthored(ids []string, authored map[string]struct{}) []string {
	if len(authored) == 0 {
		return ids
	}
	out := ids[:0:0]
	for _, id := range ids {
		if _, skip := authored[id]; skip {
			continue
		}
		out = append(out, id)
	}
	return out
}

// dropAuthoredEntries is dropAuthored for the Redis trending ZSET rows.
func dropAuthoredEntries(entries []infraredis.TrendingEntry, authored map[string]struct{}) []infraredis.TrendingEntry {
	if len(authored) == 0 {
		return entries
	}
	out := entries[:0:0]
	for _, e := range entries {
		if _, skip := authored[e.ObjectID]; skip {
			continue
		}
		out = append(out, e)
	}
	return out
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
		if !finiteScore(finalScore) {
			continue
		}

		if createdAtVal, ok := p.Payload["created_at"]; ok {
			if t, err := time.Parse(time.RFC3339, createdAtVal.GetStringValue()); err == nil {
				finalScore *= freshnessMultiplier(now, t, gamma)
			}
		}
		if !finiteScore(finalScore) {
			continue
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

// pageOf clamps [offset, offset+limit) to len(s) and returns the page plus
// its global start index (for 1-based ranks). This is the single
// paging-clamp implementation; every rung that pages a candidate list goes
// through it.
func pageOf[T any](s []T, offset, limit int) (page []T, start int) {
	start = min(offset, len(s))
	return s[start:min(start+limit, len(s))], start
}

// pageItems slices a scored list to [offset : offset+limit] and builds RecommendedItem
// values with 1-based global rank. The caller is responsible for ensuring that
// len(scored) reflects the total candidate count before slicing.
func pageItems(scored []scoredItem, offset, limit int) []RecommendedItem {
	paged, start := pageOf(scored, offset, limit)
	items := make([]RecommendedItem, len(paged))
	for i, s := range paged {
		items[i] = RecommendedItem{
			ObjectID: s.objectID,
			Score:    s.finalScore,
			Rank:     start + i + 1,
			Scored:   true,
		}
	}
	return items
}

// repageItems re-slices items built from rank 0, applying the caller's
// offset+limit and rewriting ranks. Used by hybridCold's pass-through
// branches, whose inner requests always fetch from offset 0 with limit
// offset+limit.
func repageItems(items []RecommendedItem, offset, limit int) []RecommendedItem {
	paged, start := pageOf(items, offset, limit)
	out := make([]RecommendedItem, len(paged))
	for i, it := range paged {
		it.Rank = start + i + 1
		out[i] = it
	}
	return out
}

// itemIDs extracts ObjectID strings from a RecommendedItem slice for use with blendItems.
func itemIDs(items []RecommendedItem) []string {
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ObjectID
	}
	return ids
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

// Rank scores a list of candidate items for a subject with the same hybrid
// sparse+dense blend Recommend uses (namespace alpha decides the balance) and
// returns them in descending score order. A missing side degrades rather than
// falls back: no dense vector → sparse-only, no sparse vector → dense-only at
// full weight. Only a subject with neither vector gets the whole candidate
// list back unscored in request order.
func (s *Service) Rank(ctx context.Context, req *RankRequest, ns string) (*RankResponse, error) {
	cfg, err := s.nsConfigSvc.Get(ctx, ns)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNamespaceConfigUnavailable, err)
	}
	if cfg == nil {
		return nil, ErrNamespaceNotFound
	}
	if len(req.Candidates) == 0 {
		return &RankResponse{
			SubjectID:   req.SubjectID,
			Namespace:   ns,
			Items:       []RankedItem{},
			Source:      SourceHybridRank,
			Total:       0,
			GeneratedAt: time.Now().UTC(),
		}, nil
	}

	subjectNumID, found, err := s.idmapSvc.LookupSubjectID(ctx, req.SubjectID, ns)
	if err != nil || !found {
		slog.Error("rank: get subject numeric id failed", "namespace", ns, "subject_id", req.SubjectID, "error", err)
		return s.rankFallback(req, ns), nil
	}

	physicalNamespace := qdrantPhysicalNamespace(ns, cfg)
	sparseVec, err := s.vectors.FetchSubjectVector(ctx, physicalNamespace, subjectNumID)
	if err != nil {
		slog.Error("rank: fetch subject sparse vector failed", "namespace", ns, "subject_id", req.SubjectID, "error", err)
		sparseVec = nil
	}

	// The dense side participates under the same gate as Recommend's hybrid
	// path, and shares its staleness caveat: subject dense vectors refresh on
	// the cron tick.
	var denseVec []float32
	if hybridEligible(cfg) {
		denseVec, err = s.vectors.FetchSubjectDenseVector(ctx, physicalNamespace, subjectNumID)
		if err != nil {
			slog.Error("rank: fetch subject dense vector failed", "namespace", ns, "subject_id", req.SubjectID, "error", err)
			denseVec = nil
		} else if denseVec == nil && sparseVec != nil {
			// Same silent-downgrade trace as Recommend: dense is configured
			// but this subject has no dense vector, so only sparse scores.
			slog.Warn("rank: no subject dense vector, scoring sparse-only despite dense config",
				"namespace", ns, "subject_id", req.SubjectID, "dense_source", cfg.DenseSource)
		}
	}

	if sparseVec == nil && denseVec == nil {
		slog.Info("rank: no subject vector, returning original order", "namespace", ns, "subject_id", req.SubjectID)
		return s.rankFallback(req, ns), nil
	}

	// Ranking is read-only: unknown candidates remain unscored and must not
	// mint durable mappings merely because a client asked about them.
	numIDs, err := s.idmapSvc.LookupObjectIDs(ctx, req.Candidates, ns)
	if err != nil {
		slog.Error("rank: batch id mapping failed", "namespace", ns, "error", err)
		return s.rankFallback(req, ns), nil
	}
	ids := make([]*qdrant.PointId, 0, len(numIDs))
	for _, numID := range numIDs {
		ids = append(ids, qdrant.NewIDNum(numID))
	}

	if len(ids) == 0 {
		return s.rankFallback(req, ns), nil
	}

	filter := &qdrant.Filter{
		Must: []*qdrant.Condition{
			qdrant.NewHasID(ids...),
		},
	}

	// Same eligibility as Recommend: seen items plus (when enabled) the
	// subject's own authored objects, via the same excludedObjectIDs path.
	// The MustNot rides on the candidate filter, so an excluded candidate
	// drops out of both searches and comes back Scored=false instead of
	// carrying a relevance score. Lookup failures degrade to unfiltered.
	seenItemsDays := 30
	if cfg != nil && cfg.SeenItemsDays > 0 {
		seenItemsDays = cfg.SeenItemsDays
	}
	seenItems, err := s.repo.GetSeenItems(ctx, ns, req.SubjectID, seenItemsDays)
	if err != nil {
		slog.Error("rank: get seen items failed, serving unfiltered", "namespace", ns, "subject_id", req.SubjectID, "error", err)
	}
	excluded := s.excludedObjectIDs(ctx, &Request{SubjectID: req.SubjectID, Namespace: ns}, cfg, seenItems)
	if exclusionFilter := s.buildSeenItemsFilter(ctx, ns, excluded); exclusionFilter != nil {
		filter.MustNot = exclusionFilter.MustNot
	}

	var sparseResults, denseResults []*qdrant.ScoredPoint
	sparseOK, denseOK := false, false
	if sparseVec != nil {
		sparseResults, err = s.vectors.SearchObjects(ctx, physicalNamespace, sparseVec, filter, uint64(len(ids)))
		if err != nil {
			slog.Error("rank: sparse search failed", "namespace", ns, "subject_id", req.SubjectID, "error", err)
			sparseResults = nil
		} else {
			sparseOK = true
		}
	}
	if denseVec != nil {
		denseResults, err = s.vectors.SearchObjectsDense(ctx, physicalNamespace, denseVec, filter, uint64(len(ids)))
		if err != nil {
			slog.Error("rank: dense search failed", "namespace", ns, "subject_id", req.SubjectID, "error", err)
			denseResults = nil
		} else {
			denseOK = true
		}
	}
	if !sparseOK && !denseOK {
		return s.rankFallback(req, ns), nil
	}

	// Effective alpha: the namespace blend when both sides answered, full
	// weight to the surviving side otherwise — scaling a lone side by alpha
	// would just shrink every score for no reason.
	alpha := 1.0
	switch {
	case sparseOK && denseOK:
		alpha = cfg.Alpha
	case denseOK:
		alpha = 0.0
	}

	denseDistance := ""
	if cfg != nil {
		denseDistance = cfg.DenseDistance
	}
	candidates := blendHybridScores(sparseResults, denseResults, alpha, resolveGamma(cfg), denseDistance, time.Now().UTC())

	// Every candidate the caller sent comes back: items with no overlap on
	// either side (or never upserted) score 0 and trail the scored ones in
	// request order — consistent with rankFallback, which returns the full
	// list when no subject vector exists.
	ranked := make([]RankedItem, 0, len(req.Candidates))
	present := make(map[string]struct{}, len(req.Candidates))
	for _, c := range candidates {
		present[c.objectID] = struct{}{}
		ranked = append(ranked, RankedItem{ObjectID: c.objectID, Score: c.score, Rank: len(ranked) + 1, Scored: true})
	}
	for _, c := range req.Candidates {
		if _, ok := present[c]; ok {
			continue
		}
		present[c] = struct{}{}
		ranked = append(ranked, RankedItem{ObjectID: c, Score: 0, Rank: len(ranked) + 1, Scored: false})
	}

	metrics.RecommendRequests.WithLabelValues(ns, SourceHybridRank).Inc()
	return &RankResponse{
		SubjectID:   req.SubjectID,
		Namespace:   ns,
		Items:       ranked,
		Source:      SourceHybridRank,
		Total:       len(ranked),
		GeneratedAt: time.Now().UTC(),
	}, nil
}

// rankFallback returns candidates unscored in their original order when no
// scoring could run at all. The source is SourceNoSubjectVector — named for
// its overwhelmingly common cause (the subject has neither vector); the rare
// infra-failure paths land here too because the caller-visible outcome is
// identical: keep your own ordering, nothing here is a relevance verdict.
func (s *Service) rankFallback(req *RankRequest, ns string) *RankResponse {
	items := make([]RankedItem, len(req.Candidates))
	for i, c := range req.Candidates {
		items[i] = RankedItem{ObjectID: c, Score: 0, Rank: i + 1, Scored: false}
	}
	return &RankResponse{
		SubjectID:   req.SubjectID,
		Namespace:   ns,
		Items:       items,
		Source:      SourceNoSubjectVector,
		Total:       len(items),
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
func (s *Service) DeleteObject(ctx context.Context, ns, objectID string) error {
	if s.lifecycle != nil {
		return s.lifecycle.WithWriter(ctx, ns, func(leased context.Context, lifecycle *nslifecycle.NamespaceLifecycle) error {
			return s.deleteObjectActive(leased, ns, objectID, nslifecycle.NewIncarnation(ns, lifecycle.Generation))
		})
	}
	return s.deleteObjectActive(ctx, ns, objectID, nslifecycle.NewIncarnation(ns, 1))
}

func (s *Service) deleteObjectActive(ctx context.Context, ns, objectID string, inc nslifecycle.Incarnation) error {
	// Lookup, not GetOrCreate: deleting a never-ingested object must be an
	// idempotent no-op on the vector store, not a write that mints a mapping
	// for it. A missing mapping only means there are no Qdrant points — the
	// object may still carry an objects-table metadata row (e.g. an author
	// set via PUT /objects/{id} on an object never referenced by an event),
	// which must still be dropped below.
	numID, found, lookupErr := s.idmapSvc.LookupObjectID(ctx, objectID, ns)
	var cleanupErr error
	if lookupErr != nil {
		cleanupErr = errors.Join(cleanupErr, fmt.Errorf("get numeric id: %w", lookupErr))
	}

	if lookupErr == nil && found {
		pointIDs := []*qdrant.PointId{qdrant.NewIDNum(numID)}

		if err := s.vectors.DeleteFromCollection(ctx, infraqdrant.CollectionName(inc, infraqdrant.CollectionObjects), pointIDs); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
		}

		// Dense collection is optional; DeleteFromCollection treats NotFound as
		// success, while every other failure must remain visible and retryable.
		if err := s.vectors.DeleteFromCollection(ctx, infraqdrant.CollectionName(inc, infraqdrant.CollectionObjectsDense), pointIDs); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}

	// Drop the metadata row regardless of whether a mapping existed. Without
	// this the attribution outlives the object, so re-creating the same
	// object_id later silently inherits the old author — and until then it
	// inflates author coverage and pads the exclude_authored filter with a
	// dead id.
	//
	if s.objectMeta != nil {
		if err := s.objectMeta.Delete(ctx, ns, objectID); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("delete object metadata: %w", err))
		}
	}

	return cleanupErr
}

// recCacheKey builds the per-subject cache key under the namespace-generation
// prefix owned by nslifecycle. Namespace deletion scans that same prefix, so
// the two must not each spell the key out.
func recCacheKey(ns string, generation int64, subjectID string, limit, offset int) string {
	return fmt.Sprintf("%s:%s:limit=%d:offset=%d",
		nslifecycle.MustPhysicalName(nslifecycle.KindRecommendationCache, ns, generation),
		base64.RawURLEncoding.EncodeToString([]byte(subjectID)),
		limit,
		offset,
	)
}

func namespaceGeneration(cfg *namespace.Config) int64 {
	return nslifecycle.ConfigIncarnation("", cfg).Generation()
}

// The physical-name rules live in nslifecycle so the serving path and the
// writers that created those keys cannot disagree about which generation a
// name belongs to.
func qdrantPhysicalNamespace(ns string, cfg *namespace.Config) string {
	return nslifecycle.ConfigIncarnation(ns, cfg).QdrantNamespace()
}

// mergeExclusions unions two exclusion sets, returning nil when both are
// empty so the fast path stays allocation-free.
func mergeExclusions(a, b map[string]struct{}) map[string]struct{} {
	if len(b) == 0 {
		return a
	}
	if len(a) == 0 {
		return b
	}
	merged := make(map[string]struct{}, len(a)+len(b))
	for id := range a {
		merged[id] = struct{}{}
	}
	for id := range b {
		merged[id] = struct{}{}
	}
	return merged
}
