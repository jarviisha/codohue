package redis

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/infra/metrics"
)

// StreamSpec identifies one recognized durable stream and the consumer
// groups whose progress must be protected. Namespace is empty for global
// event and catalog streams and set only for configured embed streams.
type StreamSpec struct {
	Name           string
	Kind           string
	Namespace      string
	ExpectedGroups []string
}

// RetentionResult records one completed inspection or exact trim pass.
type RetentionResult struct {
	SafeFrontier     string
	Length           int64
	Pending          int64
	Undelivered      int64
	UnexpectedGroups int
	Trimmed          int64
	DryRun           bool
}

// RetentionError identifies the fail-closed stage that prevented a pass from
// claiming progress.
type RetentionError struct {
	Stage string
	Err   error
}

func (e *RetentionError) Error() string {
	return fmt.Sprintf("stream retention %s: %v", e.Stage, e.Err)
}
func (e *RetentionError) Unwrap() error { return e.Err }

type retentionGroup struct {
	Name            string
	Pending         int64
	LastDeliveredID string
	Lag             int64
}

type retentionPending struct {
	Count    int64
	OldestID string
}

type retentionBackend interface {
	Groups(ctx context.Context, stream string) ([]retentionGroup, error)
	Pending(ctx context.Context, stream, group string) (retentionPending, error)
	Length(ctx context.Context, stream string) (int64, error)
	TrimMinID(ctx context.Context, stream, frontier string) (int64, error)
}

type redisRetentionBackend struct{ client *redis.Client }

func (b redisRetentionBackend) Groups(ctx context.Context, stream string) ([]retentionGroup, error) {
	groups, err := b.client.XInfoGroups(ctx, stream).Result()
	if err != nil {
		return nil, err
	}
	result := make([]retentionGroup, len(groups))
	for i, group := range groups {
		result[i] = retentionGroup{
			Name: group.Name, Pending: group.Pending,
			LastDeliveredID: group.LastDeliveredID, Lag: group.Lag,
		}
	}
	return result, nil
}

func (b redisRetentionBackend) Pending(ctx context.Context, stream, group string) (retentionPending, error) {
	pending, err := b.client.XPending(ctx, stream, group).Result()
	if err != nil {
		return retentionPending{}, err
	}
	return retentionPending{Count: pending.Count, OldestID: pending.Lower}, nil
}

func (b redisRetentionBackend) Length(ctx context.Context, stream string) (int64, error) {
	return b.client.XLen(ctx, stream).Result()
}

func (b redisRetentionBackend) TrimMinID(ctx context.Context, stream, frontier string) (int64, error) {
	return b.client.XTrimMinID(ctx, stream, frontier).Result()
}

// Retention computes a consumer-progress frontier and optionally executes an
// exact XTRIM MINID. Dry-run mode performs the complete inspection and emits
// metrics but never mutates Redis.
type Retention struct {
	backend retentionBackend
	dryRun  bool
}

// NewRetention creates a retention runner over a Redis client.
func NewRetention(client *redis.Client, dryRun bool) *Retention {
	return newRetentionWithBackend(redisRetentionBackend{client: client}, dryRun)
}

func newRetentionWithBackend(backend retentionBackend, dryRun bool) *Retention {
	return &Retention{backend: backend, dryRun: dryRun}
}

// RunOnce inspects every group, protects the minimum safe frontier, and uses
// exact XTRIM MINID when trimming is enabled.
func (r *Retention) RunOnce(ctx context.Context, spec StreamSpec) (RetentionResult, error) {
	result := RetentionResult{DryRun: r.dryRun}
	fail := func(stage string, err error) (RetentionResult, error) {
		metrics.StreamRetentionErrorsTotal.WithLabelValues(spec.Kind, spec.Namespace, stage).Inc()
		return RetentionResult{}, &RetentionError{Stage: stage, Err: err}
	}
	if spec.Name == "" || spec.Kind == "" || len(spec.ExpectedGroups) == 0 {
		return fail("groups", fmt.Errorf("stream name, kind, and expected groups are required"))
	}

	groups, err := r.backend.Groups(ctx, spec.Name)
	if err != nil {
		return fail("groups", err)
	}
	if len(groups) == 0 {
		return fail("groups", fmt.Errorf("stream has no consumer groups"))
	}

	expected := make(map[string]bool, len(spec.ExpectedGroups))
	for _, group := range spec.ExpectedGroups {
		expected[group] = false
	}
	var frontier string
	for _, group := range groups {
		if _, ok := expected[group.Name]; ok {
			expected[group.Name] = true
		} else {
			result.UnexpectedGroups++
		}
		if group.Pending < 0 {
			return fail("frontier", fmt.Errorf("group %q has negative pending count", group.Name))
		}

		candidate := group.LastDeliveredID
		if group.Pending > 0 {
			pending, pendingErr := r.backend.Pending(ctx, spec.Name, group.Name)
			if pendingErr != nil {
				return fail("pel", fmt.Errorf("group %q: %w", group.Name, pendingErr))
			}
			if pending.Count != group.Pending || pending.Count <= 0 || pending.OldestID == "" {
				return fail("frontier", fmt.Errorf("group %q has contradictory PEL summary", group.Name))
			}
			candidate = pending.OldestID
			result.Pending += pending.Count
		}
		if _, _, parseErr := parseStreamID(candidate); parseErr != nil {
			return fail("frontier", fmt.Errorf("group %q: %w", group.Name, parseErr))
		}
		if frontier == "" {
			frontier = candidate
		} else if cmp, compareErr := compareStreamIDs(candidate, frontier); compareErr != nil {
			return fail("frontier", compareErr)
		} else if cmp < 0 {
			frontier = candidate
		}
		if group.Lag > result.Undelivered {
			result.Undelivered = group.Lag
		}
	}
	for group, found := range expected {
		if !found {
			return fail("groups", fmt.Errorf("expected consumer group %q is missing", group))
		}
	}

	result.SafeFrontier = frontier
	result.Length, err = r.backend.Length(ctx, spec.Name)
	if err != nil {
		return fail("length", err)
	}
	metrics.StreamLength.WithLabelValues(spec.Kind, spec.Namespace).Set(float64(result.Length))
	metrics.StreamPending.WithLabelValues(spec.Kind, spec.Namespace).Set(float64(result.Pending))
	metrics.StreamUndelivered.WithLabelValues(spec.Kind, spec.Namespace).Set(float64(result.Undelivered))
	metrics.StreamUnexpectedGroups.WithLabelValues(spec.Kind, spec.Namespace).Set(float64(result.UnexpectedGroups))
	milliseconds, _, _ := parseStreamID(frontier)
	metrics.StreamRetentionFrontierMilliseconds.WithLabelValues(spec.Kind, spec.Namespace).Set(float64(milliseconds))

	if r.dryRun {
		return result, nil
	}
	result.Trimmed, err = r.backend.TrimMinID(ctx, spec.Name, frontier)
	if err != nil {
		return fail("trim", err)
	}
	metrics.StreamTrimmedTotal.WithLabelValues(spec.Kind, spec.Namespace).Add(float64(result.Trimmed))
	return result, nil
}

// RetentionRunner is the loop-facing retention surface used by binary wiring
// and tests.
type RetentionRunner interface {
	RunOnce(ctx context.Context, spec StreamSpec) (RetentionResult, error)
}

// StreamProvider returns the currently recognized streams. Embedder uses a
// dynamic provider so newly configured generation-1 namespaces are included.
type StreamProvider func(ctx context.Context) ([]StreamSpec, error)

// RunRetentionLoop performs an immediate pass, then repeats until ctx is
// cancelled. A provider or stream failure is logged and retried next tick.
func RunRetentionLoop(ctx context.Context, interval time.Duration, runner RetentionRunner, provider StreamProvider) {
	if interval <= 0 {
		interval = time.Minute
	}
	runPass := func() {
		specs, err := provider(ctx)
		if err != nil {
			slog.WarnContext(ctx, "stream retention discovery failed", "error", err)
			return
		}
		for _, spec := range specs {
			if _, err := runner.RunOnce(ctx, spec); err != nil {
				slog.WarnContext(ctx, "stream retention pass failed", "stream", spec.Name, "error", err)
			}
		}
	}
	runPass()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runPass()
		}
	}
}

func compareStreamIDs(a, b string) (int, error) {
	aMS, aSeq, err := parseStreamID(a)
	if err != nil {
		return 0, err
	}
	bMS, bSeq, err := parseStreamID(b)
	if err != nil {
		return 0, err
	}
	if aMS < bMS || (aMS == bMS && aSeq < bSeq) {
		return -1, nil
	}
	if aMS == bMS && aSeq == bSeq {
		return 0, nil
	}
	return 1, nil
}

func parseStreamID(id string) (uint64, uint64, error) {
	milliseconds, sequence, ok := strings.Cut(id, "-")
	if !ok || milliseconds == "" || sequence == "" {
		return 0, 0, fmt.Errorf("malformed stream ID %q", id)
	}
	ms, err := strconv.ParseUint(milliseconds, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("malformed stream ID %q: %w", id, err)
	}
	seq, err := strconv.ParseUint(sequence, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("malformed stream ID %q: %w", id, err)
	}
	return ms, seq, nil
}
