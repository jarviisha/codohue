package embedder

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/infra/metrics"
)

// backlogRepo is the slice of [Repository] the sampler needs. Declared as an
// interface so backlog_sampler_test.go can drive it without a real DB.
type backlogRepo interface {
	CountBacklogStates(ctx context.Context, namespace string) (BacklogStateCounts, error)
	InsertBacklogSample(ctx context.Context, namespace string, sampledAt time.Time, pending, inFlight, failed, deadLetter, streamLen int) error
	LatestBacklogSample(ctx context.Context, namespace string) (BacklogStateCounts, int, bool, error)
}

// streamObserver is the subset of *redis.Client the sampler uses to inspect
// catalog:embed:{ns} — length for the backlog row + consumer-group PEL depth
// for the consumer_lag gauge. Tests inject a fake.
type streamObserver interface {
	XLen(ctx context.Context, stream string) *redis.IntCmd
	XInfoGroups(ctx context.Context, stream string) *redis.XInfoGroupsCmd
}

// BacklogSamplerConfig bundles the runtime knobs. Zero values are filled
// with defaults at construction time.
type BacklogSamplerConfig struct {
	// Interval is how often the sampler ticks. Default 30s.
	Interval time.Duration

	// ForceWriteAfter caps the gap between two consecutive samples for a
	// namespace even when nothing changed. Keeps the timeline showing a
	// continuous line during idle hours. Default 5m (BUILD_PLAN §8 rule b).
	ForceWriteAfter time.Duration
}

// BacklogSampler periodically snapshots the live backlog (catalog_items
// counts per non-embedded state + Redis stream length) into
// catalog_backlog_samples for every catalog-enabled namespace.
//
// Skip rule (BUILD_PLAN §8 migration 014): a sample is written when the
// counts changed since last tick OR ForceWriteAfter has elapsed since the
// last sample. Identical-and-recent ticks are dropped to keep the table
// from bloating during idle hours.
type BacklogSampler struct {
	repo     backlogRepo
	redis    streamObserver
	nsLister nsLister
	cfg      BacklogSamplerConfig

	// lastSample tracks the in-memory snapshot we last wrote per namespace
	// so the skip check doesn't need a DB read every tick. The DB query in
	// LatestBacklogSample is the source of truth on startup.
	lastSample map[string]sampleSnapshot
}

type sampleSnapshot struct {
	counts    BacklogStateCounts
	streamLen int
	at        time.Time
}

// NewBacklogSampler builds a sampler. NewSampler in tests gets to inject
// fakes; production wiring lives in cmd/embedder/main.go.
func NewBacklogSampler(repo *Repository, rdb *redis.Client, nsLister nsLister, cfg BacklogSamplerConfig) *BacklogSampler {
	return newBacklogSamplerWithDeps(repo, rdb, nsLister, cfg)
}

func newBacklogSamplerWithDeps(repo backlogRepo, rdb streamObserver, nsLister nsLister, cfg BacklogSamplerConfig) *BacklogSampler {
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Second
	}
	if cfg.ForceWriteAfter <= 0 {
		cfg.ForceWriteAfter = 5 * time.Minute
	}
	return &BacklogSampler{
		repo:       repo,
		redis:      rdb,
		nsLister:   nsLister,
		cfg:        cfg,
		lastSample: make(map[string]sampleSnapshot),
	}
}

// Run blocks until ctx is cancelled, ticking the sampler every Interval. One
// missed tick is acceptable (best-effort observability); we don't catch up.
func (s *BacklogSampler) Run(ctx context.Context) {
	slog.Info("catalog backlog sampler started", "interval", s.cfg.Interval, "force_write_after", s.cfg.ForceWriteAfter)

	// Initial tick fires immediately so the timeline starts collecting
	// without waiting for the first Interval.
	s.tick(ctx)

	t := time.NewTicker(s.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("catalog backlog sampler stopped")
			return
		case <-t.C:
			s.tick(ctx)
		}
	}
}

// tick iterates every catalog-enabled namespace and records one snapshot per
// namespace subject to the skip rule. Per-namespace errors are logged and
// the loop continues — one bad namespace doesn't stall the whole sweep.
func (s *BacklogSampler) tick(ctx context.Context) {
	cfgs, err := s.nsLister.ListCatalogEnabled(ctx)
	if err != nil {
		slog.Warn("backlog sampler: list namespaces failed", "error", err)
		return
	}
	now := time.Now().UTC()
	for _, cfg := range cfgs {
		if err := s.sampleOne(ctx, cfg.Namespace, now); err != nil {
			slog.Warn("backlog sampler: sample failed", "namespace", cfg.Namespace, "error", err)
		}
	}
}

// sampleOne snapshots one namespace and writes the row if either the counts
// changed or ForceWriteAfter has elapsed since the last write.
func (s *BacklogSampler) sampleOne(ctx context.Context, namespace string, now time.Time) error {
	counts, err := s.repo.CountBacklogStates(ctx, namespace)
	if err != nil {
		return err
	}
	streamLen, err := s.streamLen(ctx, namespace)
	if err != nil {
		return err
	}

	prev, ok := s.lastSample[namespace]
	if !ok {
		// Cold start — populate from the DB so the skip rule honours
		// samples from previous sampler runs across restarts.
		dbCounts, dbStream, exists, err := s.repo.LatestBacklogSample(ctx, namespace)
		if err != nil {
			// Log + continue with no prior; worst case we write a
			// possibly-duplicate first sample.
			slog.Warn("backlog sampler: latest sample lookup failed", "namespace", namespace, "error", err)
		}
		if exists {
			// We don't know the DB row's exact age. Treat it as if it
			// was just sampled now — that way an identical first tick
			// after restart skips the write, and the ForceWriteAfter
			// timer effectively starts fresh.
			prev = sampleSnapshot{counts: dbCounts, streamLen: dbStream, at: now}
			s.lastSample[namespace] = prev
			ok = true
		}
	}

	unchanged := ok &&
		prev.counts == counts &&
		prev.streamLen == streamLen
	tooSoon := ok && now.Sub(prev.at) < s.cfg.ForceWriteAfter
	if unchanged && tooSoon {
		return nil
	}

	if err := s.repo.InsertBacklogSample(ctx, namespace, now, counts.Pending, counts.InFlight, counts.Failed, counts.DeadLetter, streamLen); err != nil {
		return err
	}
	s.lastSample[namespace] = sampleSnapshot{counts: counts, streamLen: streamLen, at: now}

	// Mirror the snapshot into Prometheus gauges so /metrics scrapes show
	// live backlog state without hitting the DB. Each non-embedded state is
	// a separate label value rather than four metrics — keeps Grafana dashboards
	// idiomatic ("sum by (state)").
	metrics.CatalogPendingItems.WithLabelValues(namespace, "pending").Set(float64(counts.Pending))
	metrics.CatalogPendingItems.WithLabelValues(namespace, "in_flight").Set(float64(counts.InFlight))
	metrics.CatalogPendingItems.WithLabelValues(namespace, "failed").Set(float64(counts.Failed))
	metrics.CatalogPendingItems.WithLabelValues(namespace, "dead_letter").Set(float64(counts.DeadLetter))

	// Consumer-lag = XINFO GROUPS pending count for the embedder consumer
	// group on this stream. Logged + zeroed on error rather than failing
	// the whole sample; the rest of the snapshot is still useful.
	if lag, err := s.consumerLag(ctx, namespace); err != nil {
		slog.Warn("backlog sampler: consumer lag lookup failed", "namespace", namespace, "error", err)
		metrics.CatalogConsumerLag.WithLabelValues(namespace).Set(0)
	} else {
		metrics.CatalogConsumerLag.WithLabelValues(namespace).Set(float64(lag))
	}
	return nil
}

// consumerLag reports the embedder consumer group's PEL depth via XINFO
// GROUPS. Returns 0 when the stream or group doesn't exist yet (cold start)
// rather than a hard error.
func (s *BacklogSampler) consumerLag(ctx context.Context, namespace string) (int64, error) {
	if s.redis == nil {
		return 0, nil
	}
	groups, err := s.redis.XInfoGroups(ctx, streamName(namespace)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, err
	}
	for _, g := range groups {
		if g.Name == defaultConsumerGroup {
			return g.Pending, nil
		}
	}
	return 0, nil
}

func (s *BacklogSampler) streamLen(ctx context.Context, namespace string) (int, error) {
	if s.redis == nil {
		return 0, nil
	}
	cmd := s.redis.XLen(ctx, streamName(namespace))
	n, err := cmd.Result()
	if err != nil {
		// Stream not yet created shows up as redis.Nil — treat as zero
		// rather than a hard failure, since the rest of the sample is
		// still useful and the stream will appear on first XADD.
		if errors.Is(err, redis.Nil) {
			return 0, nil
		}
		return 0, err
	}
	return int(n), nil
}
