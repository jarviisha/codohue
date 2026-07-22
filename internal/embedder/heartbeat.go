package embedder

import (
	"context"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// HeartbeatKey is the Redis key the embedder refreshes while it is alive.
// The admin plane reads it to report embedder liveness on /overview; the
// key's TTL is what makes a dead replica disappear on its own.
const HeartbeatKey = "codohue:embedder:heartbeat"

const (
	heartbeatInterval = 30 * time.Second
	// heartbeatTTL must exceed the interval by enough that one slow tick
	// does not make a healthy embedder look dead.
	heartbeatTTL = 90 * time.Second
)

// heartbeatWriter is the subset of *redis.Client the heartbeat needs.
type heartbeatWriter interface {
	Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
}

// Heartbeat periodically stamps HeartbeatKey so the admin plane can tell a
// running embedder from a dead one. Without it the overview page hardcoded
// "embedder OK" and reported a healthy worker even when the process was down.
type Heartbeat struct {
	redis    heartbeatWriter
	replica  string
	interval time.Duration
	ttl      time.Duration
	clock    func() time.Time
}

// NewHeartbeat builds a Heartbeat for this replica. A nil client disables it.
func NewHeartbeat(rdb *redis.Client, replica string) *Heartbeat {
	return newHeartbeatWithDeps(rdb, replica)
}

func newHeartbeatWithDeps(rdb heartbeatWriter, replica string) *Heartbeat {
	return &Heartbeat{
		redis:    rdb,
		replica:  replica,
		interval: heartbeatInterval,
		ttl:      heartbeatTTL,
		clock:    time.Now,
	}
}

// Run stamps the key immediately and then every interval until ctx is done.
func (h *Heartbeat) Run(ctx context.Context) {
	if h.redis == nil {
		slog.Info("embedder heartbeat disabled (no redis)")
		return
	}
	slog.Info("embedder heartbeat started", "key", HeartbeatKey, "interval", h.interval)

	h.beat(ctx)
	t := time.NewTicker(h.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("embedder heartbeat stopped")
			return
		case <-t.C:
			h.beat(ctx)
		}
	}
}

// beat writes the current timestamp. Failures are logged and retried on the
// next tick — a missed beat degrades to "embedder silent" on the admin page,
// which is the honest reading when Redis is unreachable anyway.
func (h *Heartbeat) beat(ctx context.Context) {
	value := h.clock().UTC().Format(time.RFC3339)
	if h.replica != "" {
		value += "|" + h.replica
	}
	if err := h.redis.Set(ctx, HeartbeatKey, value, h.ttl).Err(); err != nil {
		if ctx.Err() == nil {
			slog.WarnContext(ctx, "embedder heartbeat write failed", slog.String("error", err.Error()))
		}
	}
}
