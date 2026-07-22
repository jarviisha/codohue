package embedder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/qdrant/go-client/qdrant"

	"github.com/jarviisha/codohue/internal/core/embedstrategy"
	"github.com/jarviisha/codohue/internal/core/namespace"
	"github.com/jarviisha/codohue/internal/infra/metrics"
	infraqdrant "github.com/jarviisha/codohue/internal/infra/qdrant"
)

// denseVectorName is the named vector key inside Qdrant points. MUST match
// the constant of the same name in internal/recommend so vectors written
// by the embedder are read back by recommend without any indirection. If
// recommend ever changes its key, this value must be updated in lockstep.
const denseVectorName = "dense_interactions"

// ProcessOutcome enumerates the possible results of processing a single
// stream entry. Used by the worker to decide whether to XACK the entry.
type ProcessOutcome int

const (
	// OutcomeEmbedded — happy path; row is now state='embedded'. Worker MUST ACK.
	OutcomeEmbedded ProcessOutcome = iota

	// OutcomeDeadLetter — terminal failure (zero-norm, dim mismatch,
	// max-attempts exhausted). Row is now state='dead_letter'. Worker MUST ACK.
	OutcomeDeadLetter

	// OutcomeSkipped — entry references a row that no longer exists OR a
	// namespace that is no longer catalog-enabled. Worker MUST ACK.
	OutcomeSkipped

	// OutcomeFailed — transient failure (network error, DB blip, panic).
	// Row is now state='failed'. Worker MUST NOT ACK; XAUTOCLAIM will retry.
	OutcomeFailed
)

// ShouldAck reports whether the worker should XACK the stream entry that
// produced this outcome.
func (o ProcessOutcome) ShouldAck() bool { return o != OutcomeFailed }

// catalogItemRepo abstracts Repository for tests.
type catalogItemRepo interface {
	LoadByID(ctx context.Context, id int64) (*PendingItem, error)
	MarkInFlight(ctx context.Context, id int64) (int, error)
	MarkEmbedded(ctx context.Context, id int64, strategyID, strategyVersion string, embeddedAt time.Time, contentHash []byte) error
	MarkFailed(ctx context.Context, id int64, lastError string) error
	MarkDeadLetter(ctx context.Context, id int64, lastError string) error
}

// nsConfigGetter abstracts nsconfig.Service.Get for tests.
type nsConfigGetter interface {
	Get(ctx context.Context, namespace string) (*namespace.Config, error)
}

// idmapper abstracts idmap.Service.GetOrCreateObjectID for tests.
type idmapper interface {
	GetOrCreateObjectID(ctx context.Context, objectID, namespace string) (uint64, error)
}

// strategyBuilder abstracts embedstrategy.Registry for tests.
type strategyBuilder interface {
	Build(id, version string, p embedstrategy.Params) (embedstrategy.Strategy, error)
}

// Service performs per-item embed orchestration: load the catalog row, build
// (or look up cached) Strategy, embed, upsert Qdrant, and write the resulting
// state transition. Mapping of strategy errors to lifecycle states matches
// the table in contracts/strategy-interface.md.
type Service struct {
	repo     catalogItemRepo
	nsCfg    nsConfigGetter
	registry strategyBuilder
	idmap    idmapper

	qdrantClient   *qdrant.Client
	qdrantUpsertFn func(ctx context.Context, points *qdrant.UpsertPoints) error
	ensureCollFn   func(ctx context.Context, ns string, dim uint64, distance string) error

	clock func() time.Time

	cacheMu sync.RWMutex
	cache   map[string]cachedStrategy

	ensuredMu sync.Mutex
	ensured   map[string]struct{}

	// eventPublisher is optional. When set, the service emits one
	// CatalogItemStateChangedEvent after every successful state transition
	// so the admin plane can fan progress out to operators over SSE.
	eventPublisher CatalogEventPublisher
}

// SetEventPublisher wires a pub/sub publisher used to broadcast catalog item
// state changes. Production wiring (cmd/embedder/main.go) passes a Redis-
// backed publisher; tests inject a fake or leave it nil. Safe to call
// before Run.
func (s *Service) SetEventPublisher(p CatalogEventPublisher) { s.eventPublisher = p }

type cachedStrategy struct {
	strategy embedstrategy.Strategy
	key      string
}

// NewService wires the Service. The qdrant.Client is the production
// concrete client; tests substitute via the unexported qdrantUpsertFn /
// ensureCollFn fields.
func NewService(
	repo *Repository,
	nsCfg nsConfigGetter,
	registry strategyBuilder,
	idmap idmapper,
	qdrantClient *qdrant.Client,
) *Service {
	s := &Service{
		repo:         repo,
		nsCfg:        nsCfg,
		registry:     registry,
		idmap:        idmap,
		qdrantClient: qdrantClient,
		clock:        time.Now,
		cache:        make(map[string]cachedStrategy),
		ensured:      make(map[string]struct{}),
	}
	s.qdrantUpsertFn = func(ctx context.Context, points *qdrant.UpsertPoints) error {
		_, err := qdrantClient.Upsert(ctx, points)
		if err != nil {
			return fmt.Errorf("qdrant upsert: %w", err)
		}
		return nil
	}
	s.ensureCollFn = func(ctx context.Context, ns string, dim uint64, distance string) error {
		return infraqdrant.EnsureDenseCollections(ctx, qdrantClient, ns, dim, distance)
	}
	return s
}

// ProcessItem loads the catalog row identified by catalogItemID, embeds its
// content under the namespace's currently-active strategy, upserts the
// resulting vector to Qdrant, and writes the appropriate state transition.
//
// The returned ProcessOutcome dictates whether the worker should XACK the
// stream entry that surfaced this id.
func (s *Service) ProcessItem(ctx context.Context, catalogItemID int64) (ProcessOutcome, error) {
	item, err := s.repo.LoadByID(ctx, catalogItemID)
	if errors.Is(err, ErrItemNotFound) {
		return OutcomeSkipped, nil
	}
	if err != nil {
		return OutcomeFailed, fmt.Errorf("load catalog item: %w", err)
	}

	cfg, err := s.nsCfg.Get(ctx, item.Namespace)
	if err != nil {
		return OutcomeFailed, fmt.Errorf("load namespace config: %w", err)
	}
	if cfg == nil || cfg.DenseSource != "catalog" {
		// Namespace was switched off catalog (or deleted) between enqueue and
		// processing. ACK the entry; do not touch the row.
		return OutcomeSkipped, nil
	}

	// Idempotency short-circuit: a duplicate delivery (reaper reclaim after
	// a lost XACK, redrive of an already-processed entry) of an item that is
	// already embedded under the active strategy must not re-embed or burn
	// an attempt. Content changes reset state to 'pending' at ingest, and
	// MarkEmbedded's hash guard keeps this state truthful.
	if item.State == StateEmbedded &&
		item.StrategyID == cfg.CatalogStrategyID &&
		item.StrategyVersion == cfg.CatalogStrategyVersion {
		return OutcomeSkipped, nil
	}

	maxAttempts := cfg.CatalogMaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}

	attempt, err := s.repo.MarkInFlight(ctx, item.ID)
	if errors.Is(err, ErrItemNotFound) {
		return OutcomeSkipped, nil
	}
	if err != nil {
		return OutcomeFailed, fmt.Errorf("mark in_flight: %w", err)
	}
	s.publishItemStateChanged(ctx, item, "pending", "in_flight")

	if attempt > maxAttempts {
		s.recordFailure(item.Namespace, cfg.CatalogStrategyID, cfg.CatalogStrategyVersion, "max_attempts")
		slog.WarnContext(ctx, "catalog item dead-lettered (max attempts exceeded)",
			slog.String("namespace", item.Namespace),
			slog.String("object_id", item.ObjectID),
			slog.Int64("catalog_item_id", item.ID),
			slog.Int("attempt", attempt),
			slog.Int("max_attempts", maxAttempts),
		)
		return s.deadLetterOutcome(ctx, item,
			fmt.Sprintf("attempt %d exceeds max %d", attempt, maxAttempts))
	}

	strategy, err := s.resolveStrategy(item.Namespace, cfg)
	if err != nil {
		// Misconfiguration (unknown strategy or factory rejected params) —
		// terminal until operator fixes config; dead-letter immediately.
		s.recordFailure(item.Namespace, cfg.CatalogStrategyID, cfg.CatalogStrategyVersion, "strategy_resolve")
		return s.deadLetterOutcome(ctx, item, fmt.Sprintf("strategy resolve: %v", err))
	}

	embedStart := s.clock()
	vec, err := strategy.Embed(ctx, item.Content)
	if err != nil {
		return s.handleEmbedError(ctx, item, strategy, err)
	}

	if len(vec) != cfg.EmbeddingDim {
		s.recordFailure(item.Namespace, strategy.ID(), strategy.Version(), "dim_mismatch")
		return s.deadLetterOutcome(ctx, item,
			fmt.Sprintf("dim mismatch: produced=%d expected=%d", len(vec), cfg.EmbeddingDim))
	}

	if err := s.ensureNamespaceCollections(ctx, item.Namespace, cfg); err != nil {
		s.markFailed(ctx, item, fmt.Sprintf("ensure dense collections: %v", err))
		s.recordFailure(item.Namespace, strategy.ID(), strategy.Version(), "ensure_collections")
		return OutcomeFailed, fmt.Errorf("ensure dense collections: %w", err)
	}

	pointID, err := s.idmap.GetOrCreateObjectID(ctx, item.ObjectID, item.Namespace)
	if err != nil {
		s.markFailed(ctx, item, fmt.Sprintf("idmap: %v", err))
		s.recordFailure(item.Namespace, strategy.ID(), strategy.Version(), "idmap")
		return OutcomeFailed, fmt.Errorf("idmap: %w", err)
	}

	embeddedAt := s.clock().UTC()
	if err := s.upsertVector(ctx, item.Namespace, item.ObjectID, pointID, vec, strategy, embeddedAt); err != nil {
		// The collection may have been dropped (namespace wipe + recreate):
		// invalidate the ensure cache so the next attempt re-creates it
		// instead of dead-lettering every item until a process restart.
		s.invalidateEnsured(item.Namespace)
		s.markFailed(ctx, item, fmt.Sprintf("qdrant: %v", err))
		s.recordFailure(item.Namespace, strategy.ID(), strategy.Version(), "qdrant")
		return OutcomeFailed, fmt.Errorf("qdrant upsert: %w", err)
	}

	if err := s.repo.MarkEmbedded(ctx, item.ID, strategy.ID(), strategy.Version(), embeddedAt, item.ContentHash); err != nil {
		// Postgres write failed AFTER Qdrant succeeded. The vector is in
		// Qdrant but the row says 'in_flight' (or, for ErrStaleItem, is
		// 'pending' again with new content). Surface as transient — the
		// retry reloads the row and re-embeds whatever it holds now.
		s.recordFailure(item.Namespace, strategy.ID(), strategy.Version(), "mark_embedded")
		return OutcomeFailed, fmt.Errorf("mark embedded: %w", err)
	}
	s.publishItemStateChanged(ctx, item, "in_flight", "embedded")

	elapsed := s.clock().Sub(embedStart)
	s.recordSuccess(item.Namespace, strategy.ID(), strategy.Version(), elapsed)
	slog.DebugContext(ctx, "catalog item embedded",
		slog.String("namespace", item.Namespace),
		slog.String("object_id", item.ObjectID),
		slog.Int64("catalog_item_id", item.ID),
		slog.String("strategy_id", strategy.ID()),
		slog.String("strategy_version", strategy.Version()),
		slog.Duration("elapsed", elapsed),
	)
	return OutcomeEmbedded, nil
}

// recordSuccess emits the four "happy path" metrics: items_embedded,
// embed_duration, work_volume (V1 unit="items"). Centralised so future
// strategies can extend the unit dimension without changing every
// success path.
func (s *Service) recordSuccess(ns, strategyID, strategyVersion string, elapsed time.Duration) {
	if elapsed < 0 {
		elapsed = 0
	}
	metrics.CatalogItemsEmbeddedTotal.WithLabelValues(ns, strategyID, strategyVersion).Inc()
	metrics.CatalogEmbedDuration.WithLabelValues(ns, strategyID, strategyVersion).Observe(elapsed.Seconds())
	metrics.CatalogStrategyWorkVolumeTotal.WithLabelValues(ns, strategyID, strategyVersion, "items").Inc()
}

// recordFailure emits the embed-failure counter with a reason label.
func (s *Service) recordFailure(ns, strategyID, strategyVersion, reason string) {
	metrics.CatalogEmbedFailuresTotal.WithLabelValues(ns, strategyID, strategyVersion, reason).Inc()
}

// deadLetterOutcome writes the dead_letter transition and maps the result to
// an outcome the worker can trust: when the bookkeeping write itself fails,
// the entry must NOT be ACKed (OutcomeFailed keeps it in the PEL for the
// reaper), otherwise the row would sit 'in_flight' forever with its stream
// entry gone — recoverable only by the slow stale-in_flight sweep.
func (s *Service) deadLetterOutcome(ctx context.Context, item *PendingItem, reason string) (ProcessOutcome, error) {
	// State writes run on a detached context: a shutdown-cancelled ctx must
	// not lose the transition the item just earned.
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	if err := s.repo.MarkDeadLetter(writeCtx, item.ID, reason); err != nil {
		slog.WarnContext(ctx, "catalog mark dead_letter failed; leaving entry pending",
			slog.Int64("catalog_item_id", item.ID),
			slog.String("reason", reason),
			slog.String("error", err.Error()),
		)
		return OutcomeFailed, fmt.Errorf("mark dead_letter %d: %w", item.ID, err)
	}
	s.publishItemStateChanged(writeCtx, item, "in_flight", "dead_letter")
	return OutcomeDeadLetter, nil
}

// markFailed records a transient failure. The row stays eligible for retry
// until attempt_count exceeds max_attempts. Best-effort: the entry is not
// ACKed on the paths that call this, so a lost write only costs bookkeeping.
func (s *Service) markFailed(ctx context.Context, item *PendingItem, reason string) {
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	if err := s.repo.MarkFailed(writeCtx, item.ID, reason); err != nil {
		slog.WarnContext(ctx, "catalog mark failed (transient) failed",
			slog.Int64("catalog_item_id", item.ID),
			slog.String("reason", reason),
			slog.String("error", err.Error()),
		)
		return
	}
	s.publishItemStateChanged(writeCtx, item, "in_flight", "failed")
}

// publishItemStateChanged is the single fan-out point for state-change
// events. nil publisher = no-op (production wires it; tests usually leave
// it nil). Errors inside the publisher are swallowed there.
func (s *Service) publishItemStateChanged(ctx context.Context, item *PendingItem, from, to string) {
	if s.eventPublisher == nil || item == nil {
		return
	}
	s.eventPublisher.PublishItemStateChanged(ctx, CatalogItemStateChangedEvent{
		Namespace: item.Namespace,
		ItemID:    item.ID,
		ObjectID:  item.ObjectID,
		From:      from,
		To:        to,
		At:        s.clock().UTC(),
	})
}

// logDeadLetter emits a structured warn-level log for terminal failures so
// operators see them in stdout/stderr without having to query Postgres.
// Transient failures (ErrTransient, context cancel, unknown) are NOT logged
// here — they are noisy by nature; the metric counter alone surfaces them.
func (s *Service) logDeadLetter(ctx context.Context, item *PendingItem, strategyID, strategyVersion, reason string, err error) {
	slog.WarnContext(ctx, "catalog item dead-lettered",
		slog.String("namespace", item.Namespace),
		slog.String("object_id", item.ObjectID),
		slog.Int64("catalog_item_id", item.ID),
		slog.String("strategy_id", strategyID),
		slog.String("strategy_version", strategyVersion),
		slog.String("reason", reason),
		slog.String("error", err.Error()),
	)
}

// handleEmbedError maps Strategy.Embed errors to ProcessOutcome per the
// contract in contracts/strategy-interface.md and emits the
// catalog_embed_failures_total counter with the matching reason label.
func (s *Service) handleEmbedError(ctx context.Context, item *PendingItem, strategy embedstrategy.Strategy, err error) (ProcessOutcome, error) {
	sid, sver := strategy.ID(), strategy.Version()
	switch {
	case errors.Is(err, embedstrategy.ErrZeroNorm):
		s.recordFailure(item.Namespace, sid, sver, "zero_norm")
		s.logDeadLetter(ctx, item, sid, sver, "zero_norm", err)
		return s.deadLetterOutcome(ctx, item, err.Error())

	case errors.Is(err, embedstrategy.ErrInputTooLarge):
		s.recordFailure(item.Namespace, sid, sver, "input_too_large")
		s.logDeadLetter(ctx, item, sid, sver, "input_too_large", err)
		return s.deadLetterOutcome(ctx, item, err.Error())

	case errors.Is(err, embedstrategy.ErrDimensionMismatch):
		s.recordFailure(item.Namespace, sid, sver, "dim_mismatch")
		s.logDeadLetter(ctx, item, sid, sver, "dim_mismatch", err)
		return s.deadLetterOutcome(ctx, item, err.Error())

	case errors.Is(err, embedstrategy.ErrTransient),
		errors.Is(err, context.Canceled),
		errors.Is(err, context.DeadlineExceeded):
		s.markFailed(ctx, item, err.Error())
		s.recordFailure(item.Namespace, sid, sver, "transient")
		return OutcomeFailed, err

	default:
		// Unknown errors are conservatively treated as transient so the
		// item gets retried; persistent unknown errors will eventually
		// hit max_attempts and dead-letter via the attempt cap above.
		s.markFailed(ctx, item, err.Error())
		s.recordFailure(item.Namespace, sid, sver, "unknown")
		return OutcomeFailed, err
	}
}

// resolveStrategy returns a cached Strategy instance for the namespace.
// Cache key is (strategy_id, strategy_version, hash(params)) so a config
// change for any of those naturally invalidates the cache.
func (s *Service) resolveStrategy(ns string, cfg *namespace.Config) (embedstrategy.Strategy, error) {
	key, err := strategyCacheKey(cfg.CatalogStrategyID, cfg.CatalogStrategyVersion, cfg.CatalogStrategyParams)
	if err != nil {
		return nil, err
	}

	s.cacheMu.RLock()
	cached, ok := s.cache[ns]
	s.cacheMu.RUnlock()
	if ok && cached.key == key {
		return cached.strategy, nil
	}

	built, err := s.registry.Build(cfg.CatalogStrategyID, cfg.CatalogStrategyVersion, embedstrategy.Params(cfg.CatalogStrategyParams))
	if err != nil {
		return nil, err
	}

	s.cacheMu.Lock()
	s.cache[ns] = cachedStrategy{strategy: built, key: key}
	s.cacheMu.Unlock()
	return built, nil
}

// ensureNamespaceCollections creates the dense Qdrant collections for the
// namespace if they do not yet exist. The result is cached (keyed on
// namespace + dim + distance so a config change re-ensures) and invalidated
// by invalidateEnsured when an upsert fails — a wiped-and-recreated
// namespace must not keep hitting a cache entry for a dropped collection.
func (s *Service) ensureNamespaceCollections(ctx context.Context, ns string, cfg *namespace.Config) error {
	dim := uint64(cfg.EmbeddingDim)
	distance := cfg.DenseDistance
	if distance == "" {
		distance = "cosine"
	}
	key := fmt.Sprintf("%s|%d|%s", ns, dim, distance)

	s.ensuredMu.Lock()
	if _, done := s.ensured[key]; done {
		s.ensuredMu.Unlock()
		return nil
	}
	s.ensuredMu.Unlock()

	if err := s.ensureCollFn(ctx, ns, dim, distance); err != nil {
		return err
	}

	s.ensuredMu.Lock()
	s.ensured[key] = struct{}{}
	s.ensuredMu.Unlock()
	return nil
}

// invalidateEnsured drops every ensure-cache entry for the namespace so the
// next ProcessItem re-runs EnsureDenseCollections.
func (s *Service) invalidateEnsured(ns string) {
	s.ensuredMu.Lock()
	defer s.ensuredMu.Unlock()
	for key := range s.ensured {
		if strings.HasPrefix(key, ns+"|") {
			delete(s.ensured, key)
		}
	}
}

// upsertVector writes the dense point to {ns}_objects_dense with the V1
// payload conventions per data-model.md §4.
func (s *Service) upsertVector(ctx context.Context, ns, objectID string, pointID uint64, vec []float32, strategy embedstrategy.Strategy, embeddedAt time.Time) error {
	collection := ns + "_objects_dense"

	point := &qdrant.PointStruct{
		Id: qdrant.NewIDNum(pointID),
		Vectors: &qdrant.Vectors{
			VectorsOptions: &qdrant.Vectors_Vectors{
				Vectors: &qdrant.NamedVectors{
					Vectors: map[string]*qdrant.Vector{
						denseVectorName: qdrant.NewVectorDense(vec),
					},
				},
			},
		},
		Payload: map[string]*qdrant.Value{
			"object_id":        qdrant.NewValueString(objectID),
			"namespace":        qdrant.NewValueString(ns),
			"strategy_id":      qdrant.NewValueString(strategy.ID()),
			"strategy_version": qdrant.NewValueString(strategy.Version()),
			"embedded_at":      qdrant.NewValueString(embeddedAt.Format(time.RFC3339)),
		},
	}

	return s.qdrantUpsertFn(ctx, &qdrant.UpsertPoints{
		CollectionName: collection,
		Points:         []*qdrant.PointStruct{point},
	})
}

// strategyCacheKey produces a stable hash of (id, version, params) so the
// service can detect when a namespace's strategy config has shifted.
func strategyCacheKey(id, version string, params map[string]any) (string, error) {
	h := sha256.New()
	h.Write([]byte(id))
	h.Write([]byte{0})
	h.Write([]byte(version))
	h.Write([]byte{0})
	if params != nil {
		// json.Marshal is not stable for maps in general, but the keyspace
		// here is small and the cache invalidates conservatively (different
		// JSON output → cache miss → rebuild). For V1 that is acceptable.
		raw, err := json.Marshal(params)
		if err != nil {
			return "", fmt.Errorf("marshal strategy params for cache key: %w", err)
		}
		h.Write(raw)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
