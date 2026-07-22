package embedder

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// sweepRepo is the slice of [Repository] the sweeper needs. Declared as an
// interface so recovery_sweeper_test.go can drive it without a real DB.
type sweepRepo interface {
	ListStrandedPending(ctx context.Context, namespace string, cutoff time.Time, limit int) ([]StrandedItem, error)
	ResetStrandedInFlight(ctx context.Context, namespace string, cutoff time.Time, limit int) ([]StrandedItem, error)
}

// sweepStream is the subset of *redis.Client the sweeper uses: XINFO GROUPS
// to prove the stream is drained, XADD to republish stranded rows.
type sweepStream interface {
	XInfoGroups(ctx context.Context, stream string) *redis.XInfoGroupsCmd
	XAdd(ctx context.Context, a *redis.XAddArgs) *redis.StringCmd
}

// RecoverySweeperConfig bundles the runtime knobs. Zero values are filled
// with defaults at construction time.
type RecoverySweeperConfig struct {
	// Interval is how often the sweeper ticks. Default 2m.
	Interval time.Duration

	// PendingStaleAfter is how long a pending row must sit untouched before
	// it counts as stranded. Must comfortably exceed normal delivery latency
	// so a busy-but-healthy backlog is never republished. Default 5m.
	PendingStaleAfter time.Duration

	// InFlightStaleAfter is how long an in_flight row must sit untouched
	// before it is reset to pending. Must exceed the slowest plausible embed
	// so a live consumer is never raced. Default 15m.
	InFlightStaleAfter time.Duration

	// BatchSize caps how many rows of each kind are recovered per namespace
	// per tick. Default 500.
	BatchSize int
}

// RecoverySweeper is the safety net the rest of the catalog pipeline leans
// on: rows whose stream entry was lost — the producer's XADD failed after the
// row was persisted, an entry was ACKed but the terminal state write failed,
// or a namespace was toggled off and back on — are re-published so no item
// stays 'pending'/'in_flight' forever.
//
// To distinguish "entry lost" from "backlog is long", a namespace is only
// swept when its consumer group owes nothing: zero undelivered entries (lag)
// and an empty PEL. Anything still owed will be delivered by the consumer or
// reclaimed by the reaper; the sweeper only handles what no longer exists.
type RecoverySweeper struct {
	repo     sweepRepo
	redis    sweepStream
	nsLister nsLister
	cfg      RecoverySweeperConfig
	clock    func() time.Time
}

// NewRecoverySweeper builds a sweeper. Production wiring lives in
// cmd/embedder/main.go; tests inject fakes via newRecoverySweeperWithDeps.
func NewRecoverySweeper(repo *Repository, rdb *redis.Client, nsLister nsLister, cfg RecoverySweeperConfig) *RecoverySweeper {
	return newRecoverySweeperWithDeps(repo, rdb, nsLister, cfg)
}

func newRecoverySweeperWithDeps(repo sweepRepo, rdb sweepStream, nsLister nsLister, cfg RecoverySweeperConfig) *RecoverySweeper {
	if cfg.Interval <= 0 {
		cfg.Interval = 2 * time.Minute
	}
	if cfg.PendingStaleAfter <= 0 {
		cfg.PendingStaleAfter = 5 * time.Minute
	}
	if cfg.InFlightStaleAfter <= 0 {
		cfg.InFlightStaleAfter = 15 * time.Minute
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 500
	}
	return &RecoverySweeper{
		repo:     repo,
		redis:    rdb,
		nsLister: nsLister,
		cfg:      cfg,
		clock:    time.Now,
	}
}

// Run blocks until ctx is cancelled, ticking every Interval. The first tick
// waits a full Interval so a freshly booted worker gets to drain its streams
// before the sweeper judges anything stranded.
func (s *RecoverySweeper) Run(ctx context.Context) {
	slog.Info("catalog recovery sweeper started",
		"interval", s.cfg.Interval,
		"pending_stale_after", s.cfg.PendingStaleAfter,
		"in_flight_stale_after", s.cfg.InFlightStaleAfter,
	)

	t := time.NewTicker(s.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("catalog recovery sweeper stopped")
			return
		case <-t.C:
			s.tick(ctx)
		}
	}
}

// tick sweeps every catalog-enabled namespace. Per-namespace errors are
// logged and skipped so one bad namespace doesn't stall the whole sweep.
func (s *RecoverySweeper) tick(ctx context.Context) {
	configs, err := s.nsLister.ListCatalogNamespaces(ctx)
	if err != nil {
		slog.WarnContext(ctx, "recovery sweep: list namespaces failed", slog.String("error", err.Error()))
		return
	}
	for _, cfg := range configs {
		if ctx.Err() != nil {
			return
		}
		s.sweepNamespace(ctx, cfg.Namespace, cfg.CatalogStrategyID, cfg.CatalogStrategyVersion)
	}
}

func (s *RecoverySweeper) sweepNamespace(ctx context.Context, ns, strategyID, strategyVersion string) {
	drained, err := s.streamDrained(ctx, ns)
	if err != nil {
		slog.WarnContext(ctx, "recovery sweep: stream inspect failed",
			slog.String("namespace", ns), slog.String("error", err.Error()))
		return
	}
	if !drained {
		// Entries are still owed to consumers (or sitting in a PEL for the
		// reaper) — nothing here is stranded yet.
		return
	}

	now := s.clock()

	stranded, err := s.repo.ListStrandedPending(ctx, ns, now.Add(-s.cfg.PendingStaleAfter), s.cfg.BatchSize)
	if err != nil {
		slog.WarnContext(ctx, "recovery sweep: list stranded pending failed",
			slog.String("namespace", ns), slog.String("error", err.Error()))
		return
	}

	reset, err := s.repo.ResetStrandedInFlight(ctx, ns, now.Add(-s.cfg.InFlightStaleAfter), s.cfg.BatchSize)
	if err != nil {
		slog.WarnContext(ctx, "recovery sweep: reset stranded in_flight failed",
			slog.String("namespace", ns), slog.String("error", err.Error()))
		// Fall through: the pending rows can still be republished.
	}
	stranded = append(stranded, reset...)
	if len(stranded) == 0 {
		return
	}

	republished := 0
	for _, item := range stranded {
		if err := s.publish(ctx, ns, item, strategyID, strategyVersion, now); err != nil {
			// The row is (still) pending, so the next tick retries it.
			slog.WarnContext(ctx, "recovery sweep: republish failed",
				slog.String("namespace", ns),
				slog.Int64("catalog_item_id", item.ID),
				slog.String("error", err.Error()))
			continue
		}
		republished++
	}
	slog.InfoContext(ctx, "recovery sweep republished stranded items",
		slog.String("namespace", ns),
		slog.Int("republished", republished),
		slog.Int("reset_in_flight", len(reset)),
	)
}

// streamDrained reports whether the namespace's embed stream owes nothing to
// its consumer group: no undelivered entries and an empty PEL. A missing
// stream or group counts as drained — with no entries at all, every stale row
// is stranded by definition.
func (s *RecoverySweeper) streamDrained(ctx context.Context, ns string) (bool, error) {
	groups, err := s.redis.XInfoGroups(ctx, streamName(ns)).Result()
	if err != nil {
		if isMissingStreamErr(err) {
			return true, nil
		}
		return false, fmt.Errorf("xinfo groups %s: %w", streamName(ns), err)
	}
	for _, g := range groups {
		if g.Name != defaultConsumerGroup {
			continue
		}
		return g.Lag == 0 && g.Pending == 0, nil
	}
	// Stream exists but the group does not (e.g. Redis restarted without
	// persistence): nothing can be owed to it.
	return true, nil
}

func (s *RecoverySweeper) publish(ctx context.Context, ns string, item StrandedItem, strategyID, strategyVersion string, now time.Time) error {
	// Field layout mirrors internal/catalog's producer; only catalog_item_id
	// is authoritative (see StreamEntry).
	err := s.redis.XAdd(ctx, &redis.XAddArgs{
		Stream: streamName(ns),
		Values: map[string]any{
			"catalog_item_id":  item.ID,
			"namespace":        ns,
			"object_id":        item.ObjectID,
			"strategy_id":      strategyID,
			"strategy_version": strategyVersion,
			"enqueued_at":      now.UTC().Format(time.RFC3339Nano),
		},
	}).Err()
	if err != nil {
		return fmt.Errorf("xadd %s: %w", streamName(ns), err)
	}
	return nil
}

func isMissingStreamErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such key")
}
