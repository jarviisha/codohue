package compute

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/qdrant/go-client/qdrant"
	goredis "github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/core/idmap"
	"github.com/jarviisha/codohue/internal/infra/metrics"
	infraqdrant "github.com/jarviisha/codohue/internal/infra/qdrant"
	infraredis "github.com/jarviisha/codohue/internal/infra/redis"
	"github.com/jarviisha/codohue/internal/nsconfig"
)

type recomputer interface {
	RecomputeNamespace(ctx context.Context, namespace string, lambda float64) (subjects, objects int, err error)
}

type jobNsConfigReader interface {
	Get(ctx context.Context, namespace string) (*nsconfig.NamespaceConfig, error)
}

type jobComputeRepo interface {
	GetActiveNamespaces(ctx context.Context) ([]string, error)
	GetAllNamespaceEvents(ctx context.Context, namespace string) ([]*RawEvent, error)
	GetNamespaceEventsInWindow(ctx context.Context, namespace string, windowHours int) ([]*RawEvent, error)
}

// PhaseResult holds per-phase metrics captured during a batch run.
type PhaseResult struct {
	OK         bool
	DurationMs int
	Count1     int // phase 1: subjects; phase 2: items; phase 3: trending items
	Count2     int // phase 1: objects; phase 2: subjects (unused in phase 3)
	Error      string
}

// PhaseResults aggregates results from all three batch phases.
type PhaseResults struct {
	Phase1 *PhaseResult
	Phase2 *PhaseResult
	Phase3 *PhaseResult
}

type batchLogger interface {
	InsertBatchRunLog(ctx context.Context, namespace string, startedAt time.Time, triggerSource string) (int64, error)
	UpdateBatchRunLog(ctx context.Context, id int64, completedAt time.Time, durationMs int, subjectsProcessed int, success bool, errMsg string, logLines []LogEntry) error
	UpdateBatchRunPhases(ctx context.Context, id int64, phases PhaseResults) error
}

// Job is a periodic batch job that recomputes sparse and dense vectors for all namespaces.
type Job struct {
	service     recomputer
	nsConfigSvc jobNsConfigReader
	repo        jobComputeRepo
	batchLog    batchLogger
	redis       *goredis.Client
	interval    time.Duration

	// injectable for testing — wired to real implementations in NewJob
	ensureCollectionsFn      func(ctx context.Context, ns string) error
	ensureDenseCollectionsFn func(ctx context.Context, ns string, dim uint64, distance string) error
	upsertItemDenseFn        func(ctx context.Context, ns, strategy string, vecs map[string][]float32) error
	upsertSubjectDenseFn     func(ctx context.Context, ns, strategy string, vecs map[string][]float32) error
	storeTrendingFn          func(ctx context.Context, ns string, scores map[string]float64, ttl time.Duration) error
}

// NewJob creates a new Job with the given run interval in minutes.
// redisClient may be nil; Phase 3 (trending) is skipped when it is.
func NewJob(service *Service, nsConfigSvc *nsconfig.Service, repo *Repository, qdrantClient *qdrant.Client, idmapSvc *idmap.Service, redisClient *goredis.Client, intervalMinutes int) *Job {
	return &Job{
		service:     service,
		nsConfigSvc: nsConfigSvc,
		batchLog:    repo,
		repo:        repo,
		redis:       redisClient,
		interval:    time.Duration(intervalMinutes) * time.Minute,

		ensureCollectionsFn: func(ctx context.Context, ns string) error {
			return infraqdrant.EnsureCollections(ctx, qdrantClient, ns)
		},
		ensureDenseCollectionsFn: func(ctx context.Context, ns string, dim uint64, distance string) error {
			return infraqdrant.EnsureDenseCollections(ctx, qdrantClient, ns, dim, distance)
		},
		upsertItemDenseFn: func(ctx context.Context, ns, strategy string, vecs map[string][]float32) error {
			return UpsertItemDenseVectors(ctx, qdrantClient, idmapSvc, ns, strategy, vecs)
		},
		upsertSubjectDenseFn: func(ctx context.Context, ns, strategy string, vecs map[string][]float32) error {
			return UpsertSubjectDenseVectors(ctx, qdrantClient, idmapSvc, ns, strategy, vecs)
		},
		storeTrendingFn: func(ctx context.Context, ns string, scores map[string]float64, ttl time.Duration) error {
			return infraredis.StoreTrending(ctx, redisClient, ns, scores, ttl)
		},
	}
}

// Run starts the batch job on the configured interval (blocking).
func (j *Job) Run(ctx context.Context) {
	slog.Info("batch job started", "interval", j.interval)

	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()

	// Run immediately on startup before the first tick.
	j.runOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("batch job stopped")
			return
		case <-ticker.C:
			j.runOnce(ctx)
		}
	}
}

func (j *Job) runOnce(ctx context.Context) {
	slog.Info("batch run started")
	start := time.Now()

	namespaces, err := j.repo.GetActiveNamespaces(ctx)
	if err != nil {
		slog.Error("get active namespaces failed", "error", err)
		return
	}

	for _, ns := range namespaces {
		j.RunNamespace(ctx, ns, "cron")
	}

	elapsed := time.Since(start)
	metrics.BatchJobLagSeconds.Set(elapsed.Seconds())
	slog.Info("batch run done", "duration_ms", elapsed.Milliseconds())
}

// RunNamespace runs all batch phases for a single namespace and writes batch_run_logs.
// triggerSource is "cron" for scheduled runs or "manual" for admin-triggered runs.
func (j *Job) RunNamespace(ctx context.Context, ns, triggerSource string) {
	nsStart := time.Now()
	capture := &LogCapture{}

	var logID int64
	if j.batchLog != nil {
		var err error
		logID, err = j.batchLog.InsertBatchRunLog(ctx, ns, nsStart, triggerSource)
		if err != nil {
			slog.Warn("could not insert batch_run_log", "namespace", ns, "error", err)
		}
	}

	var runErr error
	var phases PhaseResults

	cfg, err := j.nsConfigSvc.Get(ctx, ns)
	if err != nil {
		slog.Error("get ns config failed", "namespace", ns, "error", err)
		capture.Error(fmt.Sprintf("config load failed: %v", err))
		runErr = err
	} else if cfg != nil {
		capture.Info(fmt.Sprintf("config loaded — strategy: %s, lambda: %.3f", cfg.DenseStrategy, cfg.Lambda))
	}

	if runErr == nil {
		capture.Info("phase 1 · sparse CF starting")
		t0 := time.Now()
		subjects, objects, err := j.runPhase1(ctx, ns, cfg, capture)
		durMs := int(time.Since(t0).Milliseconds())
		p := &PhaseResult{OK: err == nil, DurationMs: durMs, Count1: subjects, Count2: objects}
		if err != nil {
			slog.Error("phase 1 failed", "namespace", ns, "error", err)
			capture.Error(fmt.Sprintf("phase 1 · sparse CF failed (%dms): %v", durMs, err))
			p.Error = err.Error()
			runErr = err
		} else {
			capture.Info(fmt.Sprintf("phase 1 · sparse CF done (%dms) — subjects: %d, objects: %d", durMs, subjects, objects))
		}
		phases.Phase1 = p
	}

	if runErr == nil && cfg != nil && cfg.DenseStrategy != "" && cfg.DenseStrategy != "byoe" && cfg.DenseStrategy != "disabled" {
		capture.Info(fmt.Sprintf("phase 2 · dense starting (strategy: %s)", cfg.DenseStrategy))
		t0 := time.Now()
		items, subjectCount, err := j.runPhase2Dense(ctx, ns, cfg, capture)
		durMs := int(time.Since(t0).Milliseconds())
		p := &PhaseResult{OK: err == nil, DurationMs: durMs, Count1: items, Count2: subjectCount}
		if err != nil {
			slog.Error("phase 2 dense failed", "namespace", ns, "error", err)
			capture.Error(fmt.Sprintf("phase 2 · dense failed (%dms): %v", durMs, err))
			p.Error = err.Error()
		} else {
			capture.Info(fmt.Sprintf("phase 2 · dense done (%dms) — items: %d, subjects: %d", durMs, items, subjectCount))
		}
		phases.Phase2 = p
	} else if cfg != nil && (cfg.DenseStrategy == "byoe" || cfg.DenseStrategy == "disabled") {
		capture.Info(fmt.Sprintf("phase 2 · dense skipped (strategy: %s)", cfg.DenseStrategy))
	}

	if j.redis != nil {
		capture.Info("phase 3 · trending starting")
		t0 := time.Now()
		items, err := j.runPhase3Trending(ctx, ns, cfg, capture)
		durMs := int(time.Since(t0).Milliseconds())
		p := &PhaseResult{OK: err == nil, DurationMs: durMs, Count1: items}
		if err != nil {
			slog.Error("phase 3 trending failed", "namespace", ns, "error", err)
			capture.Error(fmt.Sprintf("phase 3 · trending failed (%dms): %v", durMs, err))
			p.Error = err.Error()
		} else {
			capture.Info(fmt.Sprintf("phase 3 · trending done (%dms) — items: %d", durMs, items))
		}
		phases.Phase3 = p
	} else {
		capture.Info("phase 3 · trending skipped (no Redis)")
	}

	subjects := 0
	if phases.Phase1 != nil {
		subjects = phases.Phase1.Count1
	}

	totalMs := int(time.Since(nsStart).Milliseconds())
	if runErr != nil {
		capture.Error(fmt.Sprintf("run failed in %dms: %v", totalMs, runErr))
	} else {
		capture.Info(fmt.Sprintf("run complete in %dms", totalMs))
	}

	if j.batchLog != nil && logID > 0 {
		now := time.Now()
		errMsg := ""
		if runErr != nil {
			errMsg = runErr.Error()
		}
		if err := j.batchLog.UpdateBatchRunLog(ctx, logID, now, totalMs, subjects, runErr == nil, errMsg, capture.Entries()); err != nil {
			slog.Warn("could not update batch_run_log", "namespace", ns, "error", err)
		}
		if err := j.batchLog.UpdateBatchRunPhases(ctx, logID, phases); err != nil {
			slog.Warn("could not update batch_run_phases", "namespace", ns, "error", err)
		}
	}
}

// runPhase1 recomputes CF sparse vectors for a namespace.
// Returns the number of subjects and objects upserted to Qdrant.
func (j *Job) runPhase1(ctx context.Context, ns string, cfg *nsconfig.NamespaceConfig, capture *LogCapture) (subjects, objects int, err error) {
	start := time.Now()

	if err := j.ensureCollectionsFn(ctx, ns); err != nil {
		return 0, 0, fmt.Errorf("ensure collections: %w", err)
	}
	capture.Info("Qdrant collections ensured")

	lambda := defaultLambda
	if cfg != nil && cfg.Lambda > 0 {
		lambda = cfg.Lambda
	}

	subjects, objects, err = j.service.RecomputeNamespace(ctx, ns, lambda)
	if err != nil {
		return 0, 0, fmt.Errorf("recompute namespace %s: %w", ns, err)
	}

	slog.Info("phase 1 sparse complete",
		"namespace", ns,
		"subjects", subjects,
		"objects", objects,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	capture.Info(fmt.Sprintf("sparse vectors computed — subjects: %d, objects: %d, lambda: %.3f", subjects, objects, lambda))
	return subjects, objects, nil
}

// item2vecLargeEventThreshold is the event count above which Item2Vec full retrain is
// expected to be slow (>30s). A warning is logged so operators can act before it becomes
// a problem in production.
const item2vecLargeEventThreshold = 500_000

// runPhase2Dense computes and upserts dense vectors for items and subjects.
//
// Retrain strategy: full retrain on every cron run — no incremental updates.
// Incremental Item2Vec (online Word2Vec) is deliberately avoided: it suffers from
// catastrophic forgetting when new items or interaction patterns shift the embedding
// space, which would silently degrade recommendation quality. Full retrain from scratch
// guarantees consistent vectors at the cost of higher per-run CPU.
//
// For corpora beyond ~500K events, consider: (a) increasing BATCH_INTERVAL_MINUTES so
// fewer retrains happen per hour, (b) switching dense_strategy to "svd" (cheaper full
// retrain), or (c) switching to "byoe" and maintaining embeddings externally.
func (j *Job) runPhase2Dense(ctx context.Context, ns string, cfg *nsconfig.NamespaceConfig, capture *LogCapture) (items, subjectCount int, err error) {
	start := time.Now()

	embeddingDim := 64
	if cfg.EmbeddingDim > 0 {
		embeddingDim = cfg.EmbeddingDim
	}
	distance := cfg.DenseDistance
	if distance == "" {
		distance = "cosine"
	}

	if err := j.ensureDenseCollectionsFn(ctx, ns, uint64(embeddingDim), distance); err != nil {
		return 0, 0, fmt.Errorf("ensure dense collections: %w", err)
	}

	events, err := j.repo.GetAllNamespaceEvents(ctx, ns)
	if err != nil {
		return 0, 0, fmt.Errorf("get namespace events: %w", err)
	}
	if len(events) == 0 {
		slog.Info("phase 2: no events, skipping dense computation", "namespace", ns)
		capture.Info("no events — dense computation skipped")
		return 0, 0, nil
	}
	capture.Info(fmt.Sprintf("fetched %d events for embedding", len(events)))

	var itemVecs map[string][]float32

	switch cfg.DenseStrategy {
	case "item2vec":
		if len(events) > item2vecLargeEventThreshold {
			slog.Warn("phase 2 item2vec: large event corpus — full retrain may be slow; consider increasing BATCH_INTERVAL_MINUTES or switching to SVD",
				"namespace", ns, "events", len(events), "threshold", item2vecLargeEventThreshold)
			capture.Warn(fmt.Sprintf("large corpus (%d events) — item2vec retrain may be slow", len(events)))
		}
		seqs := BuildInteractionSequences(events)
		i2vCfg := Item2VecConfig{Dim: embeddingDim, Window: 5, MinCount: 5, Epochs: 10, NegSamples: 5}
		itemVecs = TrainItem2Vec(seqs, i2vCfg)

	case "svd":
		itemVecs, err = SVDEmbeddings(events, embeddingDim)
		if err != nil {
			return 0, 0, fmt.Errorf("svd embeddings: %w", err)
		}
	}

	if len(itemVecs) == 0 {
		slog.Warn("phase 2: no item vectors produced", "namespace", ns, "strategy", cfg.DenseStrategy)
		capture.Warn("no item vectors produced")
		return 0, 0, nil
	}
	capture.Info(fmt.Sprintf("trained %d item vectors (dim: %d)", len(itemVecs), embeddingDim))

	if err := j.upsertItemDenseFn(ctx, ns, cfg.DenseStrategy, itemVecs); err != nil {
		return 0, 0, fmt.Errorf("upsert item dense vectors: %w", err)
	}

	subjectVecs := UserDenseVectors(events, itemVecs)
	if len(subjectVecs) > 0 {
		if err := j.upsertSubjectDenseFn(ctx, ns, cfg.DenseStrategy, subjectVecs); err != nil {
			return 0, 0, fmt.Errorf("upsert subject dense vectors: %w", err)
		}
	}
	capture.Info(fmt.Sprintf("upserted %d item + %d subject vectors to Qdrant", len(itemVecs), len(subjectVecs)))

	slog.Info("phase 2 dense complete",
		"namespace", ns,
		"strategy", cfg.DenseStrategy,
		"items", len(itemVecs),
		"subjects", len(subjectVecs),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return len(itemVecs), len(subjectVecs), nil
}

// runPhase3Trending computes trending scores for a namespace and caches them in Redis.
// Returns the number of trending items computed.
func (j *Job) runPhase3Trending(ctx context.Context, ns string, cfg *nsconfig.NamespaceConfig, capture *LogCapture) (items int, err error) {
	start := time.Now()

	windowHours := 24
	lambdaTrending := 0.1
	ttlSeconds := 600
	var actionWeights map[string]float64

	if cfg != nil {
		if cfg.TrendingWindow > 0 {
			windowHours = cfg.TrendingWindow
		}
		if cfg.LambdaTrending > 0 {
			lambdaTrending = cfg.LambdaTrending
		}
		if cfg.TrendingTTL > 0 {
			ttlSeconds = cfg.TrendingTTL
		}
		actionWeights = cfg.ActionWeights
	}

	events, err := j.repo.GetNamespaceEventsInWindow(ctx, ns, windowHours)
	if err != nil {
		return 0, fmt.Errorf("get events in window: %w", err)
	}
	if len(events) == 0 {
		slog.Info("phase 3 trending: no events in window", "namespace", ns, "window_hours", windowHours)
		capture.Info(fmt.Sprintf("no events in %dh window — trending skipped", windowHours))
		return 0, nil
	}
	capture.Info(fmt.Sprintf("scoring %d events in %dh window (λ: %.3f)", len(events), windowHours, lambdaTrending))

	scores := TrendingScores(events, actionWeights, lambdaTrending, windowHours)
	if len(scores) == 0 {
		capture.Warn("no trending scores produced")
		return 0, nil
	}

	ttl := time.Duration(ttlSeconds) * time.Second
	if err := j.storeTrendingFn(ctx, ns, scores, ttl); err != nil {
		return 0, fmt.Errorf("store trending: %w", err)
	}
	capture.Info(fmt.Sprintf("stored %d trending items to Redis (TTL: %ds)", len(scores), ttlSeconds))

	metrics.TrendingItemsTotal.WithLabelValues(ns).Set(float64(len(scores)))
	slog.Info("phase 3 trending complete",
		"namespace", ns,
		"items", len(scores),
		"window_hours", windowHours,
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return len(scores), nil
}
