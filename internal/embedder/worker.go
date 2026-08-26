package embedder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/core/namespace"
	"github.com/jarviisha/codohue/internal/core/nslifecycle"
	"github.com/jarviisha/codohue/internal/infra/metrics"
)

// Default worker tunables. Operators can override via cmd/embedder env vars.
const (
	defaultConsumerGroup  = "embedder"
	defaultPollInterval   = 30 * time.Second
	defaultReapInterval   = 60 * time.Second
	defaultMinIdleReap    = 60 * time.Second
	defaultReadBlockTime  = 5 * time.Second
	defaultReadBatchSize  = 32
	defaultReapBatchSize  = 100
	defaultReapPageBudget = 10
)

// streamClient is the subset of *redis.Client methods the worker needs.
// Defined as an interface so tests can plug in a fake without spinning up
// a real Redis instance.
type streamClient interface {
	XGroupCreateMkStream(ctx context.Context, stream, group, start string) *redis.StatusCmd
	XReadGroup(ctx context.Context, a *redis.XReadGroupArgs) *redis.XStreamSliceCmd
	XAck(ctx context.Context, stream, group string, ids ...string) *redis.IntCmd
	XAutoClaim(ctx context.Context, a *redis.XAutoClaimArgs) *redis.XAutoClaimCmd
}

// itemProcessor abstracts Service.ProcessItem for tests.
type itemProcessor interface {
	ProcessItem(ctx context.Context, catalogItemID int64) (ProcessOutcome, error)
}

type lifecycleEvaluator interface {
	EvaluateEnvelope(context.Context, string, *int64) (nslifecycle.EnvelopeDisposition, error)
}

// nsLister abstracts nsconfig.Service.ListCatalogNamespaces for tests.
type nsLister interface {
	ListCatalogNamespaces(ctx context.Context) ([]*namespace.Config, error)
}

// WorkerConfig bundles the per-replica runtime knobs.
type WorkerConfig struct {
	// ConsumerName is the consumer name registered in the Redis Streams
	// consumer group. Two replicas of cmd/embedder MUST have different
	// names so they get disjoint slices of pending entries. Defaults to
	// the OS hostname when empty.
	ConsumerName string

	// PollInterval is how often the namespace registry is refreshed from
	// nsconfig. Newly-enabled namespaces start consumers; newly-disabled
	// namespaces have their consumers cancelled.
	PollInterval time.Duration

	// ReapInterval is how often XAUTOCLAIM is invoked per active namespace
	// to reclaim entries idle in another consumer's PEL (typically a
	// crashed consumer that hasn't released its claim).
	ReapInterval time.Duration

	// MinIdleReap is the threshold passed to XAUTOCLAIM. Entries idle for
	// less than this duration are NOT reclaimed.
	MinIdleReap time.Duration

	// ReadBlockTime is the BLOCK argument to XREADGROUP. Longer values
	// reduce CPU at the cost of slightly slower shutdown response.
	ReadBlockTime time.Duration

	// ReadBatchSize is the COUNT argument to XREADGROUP per call.
	ReadBatchSize int

	// ReapBatchSize is the COUNT argument to XAUTOCLAIM per call.
	ReapBatchSize int
}

// Worker is the per-replica embedder worker. Run starts the namespace
// registry poller plus per-namespace consumer + reaper goroutines and
// blocks until ctx is cancelled or an unrecoverable error occurs.
type Worker struct {
	redis     streamClient
	service   itemProcessor
	nsLister  nsLister
	lifecycle lifecycleEvaluator
	cfg       WorkerConfig

	mu      sync.Mutex
	cancels map[string]context.CancelFunc

	wg sync.WaitGroup
}

// SetLifecycleEvaluator enables durable generation enforcement.
func (w *Worker) SetLifecycleEvaluator(evaluator lifecycleEvaluator) { w.lifecycle = evaluator }

// NewWorker constructs a Worker. Empty fields in cfg are filled with the
// package defaults.
func NewWorker(rdb *redis.Client, service *Service, nsLister nsLister, cfg WorkerConfig) *Worker {
	return newWorkerWithDeps(rdb, service, nsLister, cfg)
}

// newWorkerWithDeps lets tests inject the small streamClient interface
// instead of a real *redis.Client.
func newWorkerWithDeps(rdb streamClient, service itemProcessor, nsLister nsLister, cfg WorkerConfig) *Worker {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultPollInterval
	}
	if cfg.ReapInterval <= 0 {
		cfg.ReapInterval = defaultReapInterval
	}
	if cfg.MinIdleReap <= 0 {
		cfg.MinIdleReap = defaultMinIdleReap
	}
	if cfg.ReadBlockTime <= 0 {
		cfg.ReadBlockTime = defaultReadBlockTime
	}
	if cfg.ReadBatchSize <= 0 {
		cfg.ReadBatchSize = defaultReadBatchSize
	}
	if cfg.ReapBatchSize <= 0 {
		cfg.ReapBatchSize = defaultReapBatchSize
	}
	return &Worker{
		redis:    rdb,
		service:  service,
		nsLister: nsLister,
		cfg:      cfg,
		cancels:  make(map[string]context.CancelFunc),
	}
}

// Run starts the worker and blocks until ctx is cancelled. All spawned
// goroutines are joined before Run returns.
func (w *Worker) Run(ctx context.Context) error {
	defer w.stopAllNamespaces()
	defer w.wg.Wait()

	if err := w.refreshNamespaces(ctx); err != nil {
		slog.WarnContext(ctx, "initial namespace refresh failed", slog.String("error", err.Error()))
	}

	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("worker run: %w", ctx.Err())
		case <-ticker.C:
			if err := w.refreshNamespaces(ctx); err != nil {
				slog.WarnContext(ctx, "namespace refresh failed", slog.String("error", err.Error()))
			}
		}
	}
}

// refreshNamespaces brings the set of running per-namespace consumers in
// line with the current namespace_configs WHERE dense_source='catalog'.
// Newly-enabled namespaces gain a consumer + reaper pair; newly-disabled
// namespaces have theirs cancelled.
func (w *Worker) refreshNamespaces(ctx context.Context) error {
	cfgs, err := w.nsLister.ListCatalogNamespaces(ctx)
	if err != nil {
		return fmt.Errorf("list catalog-enabled namespaces: %w", err)
	}

	type enabledStream struct{ namespace, stream string }
	enabled := make(map[string]enabledStream, len(cfgs))
	for _, c := range cfgs {
		stream := embedStreamName(c.Namespace, c.Generation)
		enabled[stream] = enabledStream{namespace: c.Namespace, stream: stream}
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Start consumers for newly-enabled namespaces.
	for key, target := range enabled {
		if _, running := w.cancels[key]; running {
			continue
		}
		nsCtx, cancel := context.WithCancel(ctx)
		w.cancels[key] = cancel
		w.wg.Add(2)
		go func(ns, stream string) {
			defer w.wg.Done()
			w.consumePhysicalStream(nsCtx, ns, stream)
		}(target.namespace, target.stream)
		go func(ns, stream string) {
			defer w.wg.Done()
			w.reapStream(nsCtx, ns, stream)
		}(target.namespace, target.stream)
	}

	// Stop consumers for namespaces that left the enabled set.
	for ns, cancel := range w.cancels {
		if _, ok := enabled[ns]; !ok {
			cancel()
			delete(w.cancels, ns)
		}
	}

	return nil
}

// consumeStream is the per-namespace primary consumer goroutine. It reads
// new messages with XREADGROUP > and dispatches each through the service.
func (w *Worker) consumeStream(ctx context.Context, ns string) {
	w.consumePhysicalStream(ctx, ns, streamName(ns))
}

func (w *Worker) consumePhysicalStream(ctx context.Context, ns, stream string) {
	group := defaultConsumerGroup

	// The group must exist before the first XREADGROUP. Retry with backoff —
	// one failure here (e.g. the embedder booted before Redis) must not kill
	// this namespace's consumer for the life of the process: the namespace
	// stays in w.cancels, so refreshNamespaces would never restart it.
	for {
		err := w.ensureGroup(ctx, stream, group)
		if err == nil {
			break
		}
		slog.WarnContext(ctx, "ensure consumer group failed; retrying",
			slog.String("namespace", ns), slog.String("error", err.Error()))
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
	}

	slog.InfoContext(ctx, "embedder consuming", slog.String("namespace", ns), slog.String("stream", stream))

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		streams, err := w.redis.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    group,
			Consumer: w.cfg.ConsumerName,
			Streams:  []string{stream, ">"},
			Count:    int64(w.cfg.ReadBatchSize),
			Block:    w.cfg.ReadBlockTime,
		}).Result()

		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			// NOGROUP means the stream or group vanished after this consumer
			// started (namespace deleted + recreated, Redis restarted without
			// persistence). Re-create instead of retrying into the same error
			// until the process restarts.
			if strings.Contains(err.Error(), "NOGROUP") {
				if egErr := w.ensureGroup(ctx, stream, group); egErr != nil {
					slog.WarnContext(ctx, "recreate consumer group failed",
						slog.String("namespace", ns), slog.String("error", egErr.Error()))
				}
			}
			slog.WarnContext(ctx, "xreadgroup failed", slog.String("namespace", ns), slog.String("error", err.Error()))
			// Brief back-off so we don't hot-loop on persistent errors.
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}

		for _, s := range streams {
			for _, msg := range s.Messages {
				w.handleMessage(ctx, ns, stream, group, msg)
			}
		}
	}
}

// reapStream periodically reclaims entries idle in another consumer's PEL
// and re-processes them. This is how a crashed replica's pending work
// gets re-driven.
func (w *Worker) reapStream(ctx context.Context, ns, stream string) {
	// The cursor lives here, per goroutine — one per (namespace, generation)
	// stream — so a recreated namespace scans its own PEL independently of the
	// generation it replaced.
	cursor := "0-0"
	ticker := time.NewTicker(w.cfg.ReapInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		cursor = w.reapOnce(ctx, ns, stream, cursor)
	}
}

// reapOnce runs one bounded XAUTOCLAIM scan and returns the cursor the next
// tick must start from.
//
// The cursor is returned rather than reset because restarting at "0-0" every
// tick re-examines the head of the PEL forever: an entry that keeps failing at
// the front would starve every entry behind it. It resets only when Redis
// reports the terminal cursor (the scan wrapped) or NOGROUP (the group was
// recreated, so any cursor into the old PEL is meaningless); an error retains
// it, because a failed call says nothing about how far the scan had got.
func (w *Worker) reapOnce(ctx context.Context, ns, stream, cursor string) string {
	group := defaultConsumerGroup
	if cursor == "" {
		cursor = "0-0"
	}
	for page := 0; page < defaultReapPageBudget; page++ {
		msgs, next, err := w.redis.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream: stream, Group: group, Consumer: w.cfg.ConsumerName,
			MinIdle: w.cfg.MinIdleReap, Start: cursor, Count: int64(w.cfg.ReapBatchSize),
		}).Result()
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return cursor
			}
			if strings.Contains(err.Error(), "NOGROUP") {
				metrics.StreamReclaimCyclesTotal.WithLabelValues("embed", ns, "nogroup").Inc()
				return "0-0"
			}
			slog.WarnContext(ctx, "xautoclaim failed", slog.String("namespace", ns), slog.String("error", err.Error()))
			metrics.StreamReclaimCyclesTotal.WithLabelValues("embed", ns, "error").Inc()
			return cursor
		}
		for _, msg := range msgs {
			w.handleMessage(ctx, ns, stream, group, msg)
		}
		cursor = next
		if next == "0-0" || next == "" {
			metrics.StreamReclaimCyclesTotal.WithLabelValues("embed", ns, "terminal").Inc()
			return "0-0"
		}
	}
	// The cursor survives the tick, so this is a paced scan rather than a
	// stall — but it is the only signal that the PEL is deeper than one tick.
	metrics.StreamReclaimCyclesTotal.WithLabelValues("embed", ns, "budget_exhausted").Inc()
	return cursor
}

// handleMessage decodes a stream entry, dispatches it through the service,
// and ACKs (or doesn't) based on the resulting ProcessOutcome.
func (w *Worker) handleMessage(ctx context.Context, ns, stream, group string, msg redis.XMessage) {
	entry, err := DecodeStreamEntry(msg)
	if err != nil {
		// A malformed entry will never be processable. ACK to drop it
		// so it does not clog the PEL on every reaper cycle.
		slog.WarnContext(ctx, "malformed stream entry; dropping",
			slog.String("namespace", ns),
			slog.String("entry_id", msg.ID),
			slog.String("error", err.Error()),
		)
		if ackErr := w.redis.XAck(ctx, stream, group, msg.ID).Err(); ackErr != nil {
			slog.WarnContext(ctx, "xack malformed entry failed",
				slog.String("namespace", ns),
				slog.String("entry_id", msg.ID),
				slog.String("error", ackErr.Error()),
			)
		}
		return
	}
	if w.lifecycle != nil {
		disposition, lifecycleErr := w.lifecycle.EvaluateEnvelope(ctx, ns, entry.NamespaceGeneration)
		if lifecycleErr != nil {
			slog.WarnContext(ctx, "embed lifecycle check failed; leaving entry pending",
				slog.String("namespace", ns), slog.String("entry_id", msg.ID), slog.String("error", lifecycleErr.Error()))
			return
		}
		if disposition == nslifecycle.EnvelopeStale {
			metrics.StaleGenerationTotal.WithLabelValues("embed", "stale").Inc()
			if ackErr := w.redis.XAck(ctx, stream, group, msg.ID).Err(); ackErr != nil {
				slog.WarnContext(ctx, "xack stale entry failed", slog.String("namespace", ns), slog.String("entry_id", msg.ID), slog.String("error", ackErr.Error()))
			}
			return
		}
	}

	out, err := w.service.ProcessItem(ctx, entry.CatalogItemID)
	if err != nil {
		slog.WarnContext(ctx, "process catalog item failed",
			slog.String("namespace", ns),
			slog.Int64("catalog_item_id", entry.CatalogItemID),
			slog.String("error", err.Error()),
		)
	}
	if out.ShouldAck() {
		if ackErr := w.redis.XAck(ctx, stream, group, msg.ID).Err(); ackErr != nil {
			slog.WarnContext(ctx, "xack failed",
				slog.String("namespace", ns),
				slog.String("entry_id", msg.ID),
				slog.String("error", ackErr.Error()),
			)
		}
	}
}

// ensureGroup creates the consumer group if it does not already exist.
// Re-creation attempts return BUSYGROUP, which is treated as success.
func (w *Worker) ensureGroup(ctx context.Context, stream, group string) error {
	err := w.redis.XGroupCreateMkStream(ctx, stream, group, "0").Err()
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "BUSYGROUP") {
		return nil
	}
	return fmt.Errorf("xgroup create %s/%s: %w", stream, group, err)
}

func (w *Worker) stopAllNamespaces() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, cancel := range w.cancels {
		cancel()
	}
	w.cancels = nil
}

// streamName matches the producer-side helper in internal/catalog. The
// repeated literal is intentional — the constitution forbids importing
// internal/catalog from internal/embedder, so the convention lives in
// both packages with cross-references in comments.
func streamName(ns string) string { return "catalog:embed:" + ns }
