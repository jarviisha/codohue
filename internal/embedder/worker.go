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
)

// Default worker tunables. Operators can override via cmd/embedder env vars.
const (
	defaultConsumerGroup     = "embedder"
	defaultPollInterval      = 30 * time.Second
	defaultReapInterval      = 60 * time.Second
	defaultMinIdleReap       = 60 * time.Second
	defaultReadBlockTime     = 5 * time.Second
	defaultReadBatchSize     = 32
	defaultReapBatchSize     = 100
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

// nsLister abstracts nsconfig.Service.ListCatalogEnabled for tests.
type nsLister interface {
	ListCatalogEnabled(ctx context.Context) ([]*namespace.Config, error)
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
	redis    streamClient
	service  itemProcessor
	nsLister nsLister
	cfg      WorkerConfig

	mu      sync.Mutex
	cancels map[string]context.CancelFunc

	wg sync.WaitGroup
}

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
			return ctx.Err()
		case <-ticker.C:
			if err := w.refreshNamespaces(ctx); err != nil {
				slog.WarnContext(ctx, "namespace refresh failed", slog.String("error", err.Error()))
			}
		}
	}
}

// refreshNamespaces brings the set of running per-namespace consumers in
// line with the current namespace_configs WHERE catalog_enabled=true.
// Newly-enabled namespaces gain a consumer + reaper pair; newly-disabled
// namespaces have theirs cancelled.
func (w *Worker) refreshNamespaces(ctx context.Context) error {
	cfgs, err := w.nsLister.ListCatalogEnabled(ctx)
	if err != nil {
		return fmt.Errorf("list catalog-enabled namespaces: %w", err)
	}

	enabled := make(map[string]struct{}, len(cfgs))
	for _, c := range cfgs {
		enabled[c.Namespace] = struct{}{}
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	// Start consumers for newly-enabled namespaces.
	for ns := range enabled {
		if _, running := w.cancels[ns]; running {
			continue
		}
		nsCtx, cancel := context.WithCancel(ctx)
		w.cancels[ns] = cancel
		w.wg.Add(2)
		go func(ns string) {
			defer w.wg.Done()
			w.consumeStream(nsCtx, ns)
		}(ns)
		go func(ns string) {
			defer w.wg.Done()
			w.reapStream(nsCtx, ns)
		}(ns)
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
	stream := streamName(ns)
	group := defaultConsumerGroup

	if err := w.ensureGroup(ctx, stream, group); err != nil {
		slog.ErrorContext(ctx, "ensure consumer group failed", slog.String("namespace", ns), slog.String("error", err.Error()))
		return
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
func (w *Worker) reapStream(ctx context.Context, ns string) {
	stream := streamName(ns)
	group := defaultConsumerGroup
	ticker := time.NewTicker(w.cfg.ReapInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		msgs, _, err := w.redis.XAutoClaim(ctx, &redis.XAutoClaimArgs{
			Stream:   stream,
			Group:    group,
			Consumer: w.cfg.ConsumerName,
			MinIdle:  w.cfg.MinIdleReap,
			Start:    "0",
			Count:    int64(w.cfg.ReapBatchSize),
		}).Result()
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			slog.WarnContext(ctx, "xautoclaim failed", slog.String("namespace", ns), slog.String("error", err.Error()))
			continue
		}
		for _, msg := range msgs {
			w.handleMessage(ctx, ns, stream, group, msg)
		}
	}
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
		_ = w.redis.XAck(ctx, stream, group, msg.ID).Err()
		return
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
