package compute

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/qdrant/go-client/qdrant"
	goredis "github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/core/batchrun"
	"github.com/jarviisha/codohue/internal/core/idmap"
	"github.com/jarviisha/codohue/internal/core/namespace"
	"github.com/jarviisha/codohue/internal/infra/metrics"
	infraqdrant "github.com/jarviisha/codohue/internal/infra/qdrant"
	infraredis "github.com/jarviisha/codohue/internal/infra/redis"
)

type recomputer interface {
	RecomputeNamespace(ctx context.Context, namespace string, lambda float64) (subjects, objects int, err error)
}

type jobNsConfigReader interface {
	Get(ctx context.Context, namespace string) (*namespace.Config, error)
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
	InsertBatchRunLog(ctx context.Context, namespace string, startedAt time.Time, triggerSource batchrun.TriggerSource) (int64, error)
	UpdateBatchRunLog(ctx context.Context, id int64, completedAt time.Time, durationMs int, subjectsProcessed int, success bool, errMsg string, logLines []LogEntry) error
	UpdateBatchRunPhases(ctx context.Context, id int64, phases PhaseResults) error
	GetCancelRequested(ctx context.Context, id int64) (bool, error)
}

// BatchRunObserver receives lifecycle callbacks during a batch run. The admin
// plane wires this to its in-process event bus so SSE handlers can forward
// run progress to operators in real time. All callbacks fire from the cron
// goroutine — keep them cheap; the bus drops to slow subscribers itself.
type BatchRunObserver interface {
	OnRunStarted(runID int64, namespace string, triggerSource batchrun.TriggerSource)
	OnPhaseStarted(runID int64, namespace string, phase int)
	OnPhaseCompleted(runID int64, namespace string, phase int, result PhaseResult)
	OnLogLine(runID int64, namespace string, entry LogEntry)
	OnRunCompleted(runID int64, namespace string, success bool, errMsg string)
	OnRunCancelled(runID int64, namespace string)
}

// operatorCancelledMessage is a local alias of [batchrun.OperatorCancelledMessage]
// so callers in this file can use a short identifier. The canonical literal
// lives in internal/core/batchrun.
const operatorCancelledMessage = batchrun.OperatorCancelledMessage

// Job is a periodic batch job that recomputes sparse and dense vectors for all namespaces.
type Job struct {
	service     recomputer
	nsConfigSvc jobNsConfigReader
	repo        jobComputeRepo
	batchLog    batchLogger
	redis       *goredis.Client
	interval    time.Duration
	observer    BatchRunObserver // optional; nil = no-op

	// injectable for testing — wired to real implementations in NewJob
	tryLockFn                func(ctx context.Context, ns string) (release func(), ok bool, err error)
	lockFn                   func(ctx context.Context, ns string) (release func(), err error)
	finalizeOrphansFn        func(ctx context.Context, cutoff time.Time) (int64, error)
	ensureCollectionsFn      func(ctx context.Context, ns string) error
	ensureDenseCollectionsFn func(ctx context.Context, ns string, dim uint64, distance string) error
	upsertItemDenseFn        func(ctx context.Context, ns, strategy string, vecs map[string][]float32) error
	upsertSubjectDenseFn     func(ctx context.Context, ns, strategy string, vecs map[string][]float32) error
	cleanupItemDenseFn       func(ctx context.Context, ns string, keepIDs []string) (int, error)
	cleanupSubjectDenseFn    func(ctx context.Context, ns string, keepIDs []string) (int, error)
	fetchItemDenseFn         func(ctx context.Context, ns string, objectIDs []string) (map[string][]float32, error)
	storeTrendingFn          func(ctx context.Context, ns string, scores map[string]float64, ttl time.Duration) error
}

// SetObserver attaches a BatchRunObserver — the admin bridge wires this to
// its event bus at startup. Passing nil clears the observer. Safe to call
// before Run starts; not safe to swap observers while a run is in flight.
func (j *Job) SetObserver(o BatchRunObserver) { j.observer = o }

// NewJob creates a new Job with the given run interval in minutes.
// redisClient may be nil; Phase 3 (trending) is skipped when it is.
func NewJob(service *Service, nsConfigSvc jobNsConfigReader, repo *Repository, qdrantClient *qdrant.Client, idmapSvc *idmap.Service, redisClient *goredis.Client, intervalMinutes int) *Job {
	return &Job{
		service:     service,
		nsConfigSvc: nsConfigSvc,
		batchLog:    repo,
		repo:        repo,
		redis:       redisClient,
		interval:    time.Duration(intervalMinutes) * time.Minute,

		tryLockFn:         repo.TryLockNamespace,
		lockFn:            repo.LockNamespace,
		finalizeOrphansFn: repo.FinalizeOrphanRuns,
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
		cleanupItemDenseFn: func(ctx context.Context, ns string, keepIDs []string) (int, error) {
			return CleanupStaleItemDensePoints(ctx, qdrantClient, idmapSvc, ns, keepIDs)
		},
		cleanupSubjectDenseFn: func(ctx context.Context, ns string, keepIDs []string) (int, error) {
			return CleanupStaleSubjectDensePoints(ctx, qdrantClient, idmapSvc, ns, keepIDs)
		},
		fetchItemDenseFn: func(ctx context.Context, ns string, objectIDs []string) (map[string][]float32, error) {
			return FetchItemDenseVectors(ctx, qdrantClient, idmapSvc, ns, objectIDs)
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

// orphanRunCutoff is how old an open batch_run_logs row must be before the
// orphan sweep finalizes it. Must exceed the longest plausible run so an
// in-flight run in another process is never closed under it.
const orphanRunCutoff = time.Hour

func (j *Job) runOnce(ctx context.Context) {
	slog.Info("batch run started")
	start := time.Now()

	// Close rows abandoned by a crashed/redeployed process; without this,
	// phantom "running" rows block retry (409) until retention deletes them.
	if j.finalizeOrphansFn != nil {
		if n, err := j.finalizeOrphansFn(ctx, time.Now().Add(-orphanRunCutoff)); err != nil {
			slog.Warn("finalize orphan batch runs failed", "error", err)
		} else if n > 0 {
			slog.Warn("finalized orphaned batch runs", "count", n)
		}
	}

	namespaces, err := j.repo.GetActiveNamespaces(ctx)
	if err != nil {
		slog.Error("get active namespaces failed", "error", err)
		return
	}

	for _, ns := range namespaces {
		if err := j.RunNamespace(ctx, ns, batchrun.TriggerCron); err != nil {
			if errors.Is(err, batchrun.ErrRunInProgress) {
				slog.Info("skipping namespace: run already in progress", "namespace", ns)
			} else {
				slog.Error("run namespace failed", "namespace", ns, "error", err)
			}
		}
	}

	elapsed := time.Since(start)
	metrics.BatchJobLagSeconds.Set(elapsed.Seconds())
	slog.Info("batch run done", "duration_ms", elapsed.Milliseconds())
}

// RunNamespace runs all batch phases for a single namespace synchronously
// and writes batch_run_logs. triggerSource is the typed enum from
// core/batchrun so the caller cannot pass an unconstrained string by
// accident. Returns batchrun.ErrRunInProgress when another process (or this
// one) already holds the namespace's compute lock.
//
// Between phases the job polls batch_run_logs.cancel_requested; when set, it
// finalizes the row with error_message="operator_cancelled" and returns
// without running the remaining phases. Mid-phase cancel is intentionally
// unsupported — see BUILD_PLAN §9.2.
func (j *Job) RunNamespace(ctx context.Context, ns string, triggerSource batchrun.TriggerSource) error {
	release, ok, err := j.tryLock(ctx, ns)
	if err != nil {
		return fmt.Errorf("namespace lock %s: %w", ns, err)
	}
	if !ok {
		return fmt.Errorf("%w: %s", batchrun.ErrRunInProgress, ns)
	}
	defer release()

	j.runNamespaceLocked(ctx, ns, triggerSource, 0)
	return nil
}

// StartNamespaceRun acquires the namespace lock, inserts the running
// batch_run_logs row, and executes the run in a background goroutine
// detached from ctx — the caller's request can end (or be cancelled)
// without aborting the run. Returns the run id for the Location header.
// The lock is held for exactly the run's lifetime; timeout bounds the run.
func (j *Job) StartNamespaceRun(ctx context.Context, ns string, triggerSource batchrun.TriggerSource, timeout time.Duration) (int64, error) {
	release, ok, err := j.tryLock(ctx, ns)
	if err != nil {
		return 0, fmt.Errorf("namespace lock %s: %w", ns, err)
	}
	if !ok {
		return 0, fmt.Errorf("%w: %s", batchrun.ErrRunInProgress, ns)
	}

	var logID int64
	if j.batchLog != nil {
		logID, err = j.batchLog.InsertBatchRunLog(ctx, ns, time.Now(), triggerSource)
		if err != nil {
			release()
			return 0, fmt.Errorf("insert batch_run_log: %w", err)
		}
	}

	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	go func() {
		defer cancel()
		defer release()
		j.runNamespaceLocked(runCtx, ns, triggerSource, logID)
	}()
	return logID, nil
}

// tryLock delegates to the injected lock fn; a Job built without one (unit
// tests) runs unlocked.
func (j *Job) tryLock(ctx context.Context, ns string) (release func(), ok bool, err error) {
	if j.tryLockFn == nil {
		return func() {}, true, nil
	}
	return j.tryLockFn(ctx, ns)
}

// LockNamespace acquires the namespace compute lock, blocking until the
// current holder (a cron tick or manual run) releases it. The admin
// namespace wipe takes this before deleting so a run in flight can never
// re-upsert Qdrant collections after the wipe finishes.
func (j *Job) LockNamespace(ctx context.Context, ns string) (release func(), err error) {
	if j.lockFn == nil {
		return func() {}, nil
	}
	return j.lockFn(ctx, ns)
}

// runNamespaceLocked executes the three phases and finalizes the run row.
// The caller must hold the namespace lock. logID > 0 means the row was
// already inserted (async path); 0 inserts it here.
func (j *Job) runNamespaceLocked(ctx context.Context, ns string, triggerSource batchrun.TriggerSource, logID int64) {
	nsStart := time.Now()
	capture := &LogCapture{}

	if logID == 0 && j.batchLog != nil {
		var err error
		logID, err = j.batchLog.InsertBatchRunLog(ctx, ns, nsStart, triggerSource)
		if err != nil {
			slog.Warn("could not insert batch_run_log", "namespace", ns, "error", err)
		}
	}

	// Forward every captured log line to the observer so the admin SSE
	// stream sees them as they happen, not just when the run finalizes.
	if j.observer != nil && logID > 0 {
		runID := logID
		capture.SetOnEntry(func(e LogEntry) { j.observer.OnLogLine(runID, ns, e) })
		j.observer.OnRunStarted(logID, ns, triggerSource)
	}

	var runErr error
	var cancelled bool
	var phases PhaseResults

	cfg, err := j.nsConfigSvc.Get(ctx, ns)
	if err != nil {
		slog.Error("get ns config failed", "namespace", ns, "error", err)
		capture.Error(fmt.Sprintf("config load failed: %v", err))
		runErr = err
	} else if cfg != nil {
		capture.Info(fmt.Sprintf("config loaded — dense_source: %s, lambda: %.3f", cfg.DenseSource, cfg.Lambda))
	}

	if runErr == nil {
		phases.Phase1 = j.executePhase(ctx, logID, ns, 1, "sparse CF", capture, func() (int, int, error) {
			return j.runPhase1(ctx, ns, cfg, capture)
		})
		if !phases.Phase1.OK {
			runErr = errors.New(phases.Phase1.Error)
		}
	}

	if runErr == nil && j.checkCancelBetweenPhases(ctx, logID, 1, capture) {
		cancelled = true
	}

	if !cancelled && runErr == nil && cfg != nil && phase2Runs(cfg.DenseSource) {
		phases.Phase2 = j.executePhase(ctx, logID, ns, 2, fmt.Sprintf("dense (%s)", cfg.DenseSource), capture, func() (int, int, error) {
			return j.runPhase2Dense(ctx, ns, cfg, capture)
		})
		// Phase 2 failure is logged but does not abort the run — phase 3 still
		// runs because trending and dense are independent surfaces.
	} else if !cancelled && cfg != nil {
		capture.Info(fmt.Sprintf("phase 2 · dense skipped (dense_source: %s)", cfg.DenseSource))
	}

	if !cancelled && j.checkCancelBetweenPhases(ctx, logID, 2, capture) {
		cancelled = true
	}

	if !cancelled && j.redis != nil {
		phases.Phase3 = j.executePhase1Arg(ctx, logID, ns, 3, "trending", capture, func() (int, error) {
			return j.runPhase3Trending(ctx, ns, cfg, capture)
		})
		// Unlike phase 2 (dense is an optional surface), a failed trending
		// phase folds into the run status — an all-green run list must mean
		// every phase that ran actually succeeded.
		if !phases.Phase3.OK && runErr == nil {
			runErr = errors.New(phases.Phase3.Error)
		}
	} else if !cancelled {
		capture.Info("phase 3 · trending skipped (no Redis)")
	}

	subjects := 0
	if phases.Phase1 != nil {
		subjects = phases.Phase1.Count1
	}

	totalMs := int(time.Since(nsStart).Milliseconds())
	errMsg := ""
	success := runErr == nil && !cancelled
	switch {
	case cancelled:
		errMsg = operatorCancelledMessage
		capture.Warn(fmt.Sprintf("run cancelled by operator after %dms", totalMs))
	case runErr != nil:
		errMsg = runErr.Error()
		capture.Error(fmt.Sprintf("run failed in %dms: %v", totalMs, runErr))
	default:
		capture.Info(fmt.Sprintf("run complete in %dms", totalMs))
	}

	if j.batchLog != nil && logID > 0 {
		// Finalize on a context detached from the run's: a cancelled or
		// timed-out run must still close its row, or it stays "running"
		// forever (blocking retry with 409 until retention deletes it).
		finCtx, cancelFin := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancelFin()
		now := time.Now()
		if err := j.batchLog.UpdateBatchRunLog(finCtx, logID, now, totalMs, subjects, success, errMsg, capture.Entries()); err != nil {
			slog.Warn("could not update batch_run_log", "namespace", ns, "error", err)
		}
		if err := j.batchLog.UpdateBatchRunPhases(finCtx, logID, phases); err != nil {
			slog.Warn("could not update batch_run_phases", "namespace", ns, "error", err)
		}
	}

	if j.observer != nil && logID > 0 {
		if cancelled {
			j.observer.OnRunCancelled(logID, ns)
		} else {
			j.observer.OnRunCompleted(logID, ns, success, errMsg)
		}
	}
}

// executePhase wraps a two-count phase (subjects/objects or items/subjects)
// with start/complete observer notifications, timing, and structured logging.
func (j *Job) executePhase(ctx context.Context, runID int64, ns string, phase int, label string, capture *LogCapture, run func() (int, int, error)) *PhaseResult {
	_ = ctx
	capture.Info(fmt.Sprintf("phase %d · %s starting", phase, label))
	if j.observer != nil && runID > 0 {
		j.observer.OnPhaseStarted(runID, ns, phase)
	}
	t0 := time.Now()
	c1, c2, err := run()
	durMs := int(time.Since(t0).Milliseconds())
	p := PhaseResult{OK: err == nil, DurationMs: durMs, Count1: c1, Count2: c2}
	if err != nil {
		slog.Error("phase failed", "phase", phase, "namespace", ns, "error", err)
		capture.Error(fmt.Sprintf("phase %d · %s failed (%dms): %v", phase, label, durMs, err))
		p.Error = err.Error()
	} else {
		capture.Info(fmt.Sprintf("phase %d · %s done (%dms) — %d / %d", phase, label, durMs, c1, c2))
	}
	if j.observer != nil && runID > 0 {
		j.observer.OnPhaseCompleted(runID, ns, phase, p)
	}
	return &p
}

// executePhase1Arg is the single-count variant used by phase 3 (trending).
func (j *Job) executePhase1Arg(ctx context.Context, runID int64, ns string, phase int, label string, capture *LogCapture, run func() (int, error)) *PhaseResult {
	_ = ctx
	capture.Info(fmt.Sprintf("phase %d · %s starting", phase, label))
	if j.observer != nil && runID > 0 {
		j.observer.OnPhaseStarted(runID, ns, phase)
	}
	t0 := time.Now()
	c1, err := run()
	durMs := int(time.Since(t0).Milliseconds())
	p := PhaseResult{OK: err == nil, DurationMs: durMs, Count1: c1}
	if err != nil {
		slog.Error("phase failed", "phase", phase, "namespace", ns, "error", err)
		capture.Error(fmt.Sprintf("phase %d · %s failed (%dms): %v", phase, label, durMs, err))
		p.Error = err.Error()
	} else {
		capture.Info(fmt.Sprintf("phase %d · %s done (%dms) — items: %d", phase, label, durMs, c1))
	}
	if j.observer != nil && runID > 0 {
		j.observer.OnPhaseCompleted(runID, ns, phase, p)
	}
	return &p
}

// checkCancelBetweenPhases polls cancel_requested between phases. Returns
// true when the operator has asked to stop; the caller skips the remaining
// phases and finalizes the row as cancelled.
func (j *Job) checkCancelBetweenPhases(ctx context.Context, runID int64, afterPhase int, capture *LogCapture) bool {
	if j.batchLog == nil || runID == 0 {
		return false
	}
	requested, err := j.batchLog.GetCancelRequested(ctx, runID)
	if err != nil {
		slog.Warn("check cancel_requested failed", "id", runID, "error", err)
		return false
	}
	if requested {
		capture.Warn(fmt.Sprintf("cancel requested after phase %d — stopping", afterPhase))
	}
	return requested
}

// runPhase1 recomputes CF sparse vectors for a namespace.
// Returns the number of subjects and objects upserted to Qdrant.
func (j *Job) runPhase1(ctx context.Context, ns string, cfg *namespace.Config, capture *LogCapture) (subjects, objects int, err error) {
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

// phase2Runs reports whether the dense phase has work to do for a dense_source.
//
// "item2vec" / "svd" train item vectors and then mean-pool subject vectors.
// "catalog" does not train — cmd/embedder owns {ns}_objects_dense — but the
// phase still runs to derive subject vectors from those item vectors, because
// nothing else in the system writes {ns}_subjects_dense in that mode.
// "byoe" and "disabled" leave both collections to the client.
func phase2Runs(denseSource string) bool {
	switch denseSource {
	case "item2vec", "svd", "catalog":
		return true
	default:
		return false
	}
}

// runPhase2Dense computes and upserts dense vectors for items and subjects.
//
// Item vector ownership depends on dense_source: "item2vec" and "svd" train
// them here, "catalog" reads back what cmd/embedder already wrote. Subject
// vectors are always derived here by mean-pooling, and are the reason this
// phase runs at all under "catalog".
//
// Retrain strategy: full retrain on every cron run — no incremental updates.
// Incremental Item2Vec (online Word2Vec) is deliberately avoided: it suffers from
// catastrophic forgetting when new items or interaction patterns shift the embedding
// space, which would silently degrade recommendation quality. Full retrain from scratch
// guarantees consistent vectors at the cost of higher per-run CPU.
//
// For corpora beyond ~500K events, consider: (a) increasing CODOHUE_BATCH_INTERVAL_MINUTES so
// fewer retrains happen per hour, (b) switching dense_source to "svd" (cheaper full
// retrain), or (c) switching to "byoe" and maintaining embeddings externally.
func (j *Job) runPhase2Dense(ctx context.Context, ns string, cfg *namespace.Config, capture *LogCapture) (items, subjectCount int, err error) {
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

	// trained is false when the item vectors came from cmd/embedder rather than
	// from this run — in that case they must not be written back.
	var itemVecs map[string][]float32
	trained := true

	switch cfg.DenseSource {
	case "item2vec":
		if len(events) > item2vecLargeEventThreshold {
			slog.Warn("phase 2 item2vec: large event corpus — full retrain may be slow; consider increasing CODOHUE_BATCH_INTERVAL_MINUTES or switching to SVD",
				"namespace", ns, "events", len(events), "threshold", item2vecLargeEventThreshold)
			capture.Warn(fmt.Sprintf("large corpus (%d events) — item2vec retrain may be slow", len(events)))
		}
		seqs := BuildInteractionSequences(events)
		i2vCfg := Item2VecConfig{Dim: embeddingDim, Window: 5, MinCount: 5, Epochs: 10, NegSamples: 5}
		itemVecs = TrainItem2Vec(seqs, i2vCfg)

	case "svd":
		lambda := defaultLambda
		if cfg.Lambda > 0 {
			lambda = cfg.Lambda
		}
		itemVecs, err = SVDEmbeddings(events, embeddingDim, lambda)
		if err != nil {
			return 0, 0, fmt.Errorf("svd embeddings: %w", err)
		}

	case "catalog":
		// cmd/embedder owns {ns}_objects_dense here. Only the items that were
		// actually interacted with can contribute to a subject's mean, so we
		// load exactly those rather than scanning the whole collection.
		trained = false
		interacted := interactedObjectIDs(events)
		itemVecs, err = j.fetchItemDenseFn(ctx, ns, interacted)
		if err != nil {
			return 0, 0, fmt.Errorf("fetch catalog item dense vectors: %w", err)
		}
		capture.Info(fmt.Sprintf("loaded %d/%d interacted item vectors from catalog embeddings",
			len(itemVecs), len(interacted)))
	}

	if len(itemVecs) == 0 {
		if trained {
			slog.Warn("phase 2: no item vectors produced", "namespace", ns, "strategy", cfg.DenseSource)
			capture.Warn("no item vectors produced")
		} else {
			// Every interacted object is still pending/failed in the embedder,
			// so there is nothing to pool from yet.
			slog.Warn("phase 2: no catalog item vectors available yet", "namespace", ns)
			capture.Warn("no catalog embeddings for interacted items yet — subject vectors unchanged")
		}
		return 0, 0, nil
	}

	if trained {
		capture.Info(fmt.Sprintf("trained %d item vectors (dim: %d)", len(itemVecs), embeddingDim))
		if err := j.upsertItemDenseFn(ctx, ns, cfg.DenseSource, itemVecs); err != nil {
			return 0, 0, fmt.Errorf("upsert item dense vectors: %w", err)
		}
	}

	subjectVecs := UserDenseVectors(events, itemVecs)
	if len(subjectVecs) > 0 {
		if err := j.upsertSubjectDenseFn(ctx, ns, cfg.DenseSource, subjectVecs); err != nil {
			return 0, 0, fmt.Errorf("upsert subject dense vectors: %w", err)
		}
	}
	if trained {
		capture.Info(fmt.Sprintf("upserted %d item + %d subject vectors to Qdrant", len(itemVecs), len(subjectVecs)))
	} else {
		capture.Info(fmt.Sprintf("upserted %d subject vectors to Qdrant (item vectors owned by embedder)", len(subjectVecs)))
	}

	// Sweep out points this full retrain no longer produced. Only when this
	// run owns them: under "catalog", {ns}_objects_dense belongs to
	// cmd/embedder, and subjectVecs covers only subjects whose interacted
	// items happen to be embedded — deleting the rest would drop vectors
	// that are still valid. Best-effort, like the phase 1 sweep.
	if trained {
		if j.cleanupItemDenseFn != nil {
			if n, err := j.cleanupItemDenseFn(ctx, ns, mapKeys(itemVecs)); err != nil {
				slog.Warn("stale item dense cleanup failed", "namespace", ns, "error", err)
			} else if n > 0 {
				capture.Info(fmt.Sprintf("removed %d stale item dense vectors", n))
			}
		}
		if len(subjectVecs) > 0 && j.cleanupSubjectDenseFn != nil {
			if n, err := j.cleanupSubjectDenseFn(ctx, ns, mapKeys(subjectVecs)); err != nil {
				slog.Warn("stale subject dense cleanup failed", "namespace", ns, "error", err)
			} else if n > 0 {
				capture.Info(fmt.Sprintf("removed %d stale subject dense vectors", n))
			}
		}
	}

	slog.Info("phase 2 dense complete",
		"namespace", ns,
		"strategy", cfg.DenseSource,
		"items", len(itemVecs),
		"subjects", len(subjectVecs),
		"duration_ms", time.Since(start).Milliseconds(),
	)
	return len(itemVecs), len(subjectVecs), nil
}

// runPhase3Trending computes trending scores for a namespace and caches them in Redis.
// Returns the number of trending items computed.
func (j *Job) runPhase3Trending(ctx context.Context, ns string, cfg *namespace.Config, capture *LogCapture) (items int, err error) {
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

// mapKeys returns the keys of a string-keyed map, in map order.
func mapKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
