package embedder

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// CatalogItemStateChangedEvent is the JSON-marshalled payload published to
// Redis pub/sub when an item transitions state. cmd/admin's catalog bridge
// subscribes to `codohue:catalog-events:*` and republishes onto the in-process
// event bus so SSE handlers can fan it out to operators in real time.
//
// `From` is best-effort: the previous state is not always known at publish
// time (LoadByID returns the row before MarkInFlight runs, so we know "from
// pending → in_flight"; for subsequent transitions we only know "from
// in_flight → {embedded, failed, dead_letter}").
type CatalogItemStateChangedEvent struct {
	Kind      string    `json:"kind"` // always "item_state_changed"
	Namespace string    `json:"namespace"`
	ItemID    int64     `json:"item_id"`
	ObjectID  string    `json:"object_id"`
	From      string    `json:"from,omitempty"`
	To        string    `json:"to"`
	At        time.Time `json:"at"`
}

// CatalogEventPublisher publishes catalog state-change events to a transport
// the admin plane can subscribe to. Production uses Redis pub/sub; tests
// inject an in-memory fake.
type CatalogEventPublisher interface {
	PublishItemStateChanged(ctx context.Context, ev CatalogItemStateChangedEvent)
}

// CatalogEventChannel computes the Redis pub/sub channel name for a
// namespace. Centralised so the embedder publisher + admin subscriber agree
// on the wire — keep them in lockstep.
func CatalogEventChannel(namespace string) string {
	return "codohue:catalog-events:" + namespace
}

// CatalogEventChannelPattern matches every namespace's catalog event channel.
// The admin bridge psubscribes to this pattern so it doesn't need to track
// which namespaces are currently enabled.
const CatalogEventChannelPattern = "codohue:catalog-events:*"

// redisPubsubPublisher is the production [CatalogEventPublisher]. Publish
// failures are logged + dropped — pub/sub is best-effort observability, not
// a critical path; we will not fail an embed because Redis is briefly down.
type redisPubsubPublisher struct {
	rdb *redis.Client
}

// NewRedisCatalogEventPublisher wraps a Redis client as a CatalogEventPublisher.
func NewRedisCatalogEventPublisher(rdb *redis.Client) CatalogEventPublisher {
	return &redisPubsubPublisher{rdb: rdb}
}

func (p *redisPubsubPublisher) PublishItemStateChanged(ctx context.Context, ev CatalogItemStateChangedEvent) {
	if ev.Kind == "" {
		ev.Kind = "item_state_changed"
	}
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		slog.WarnContext(ctx, "catalog event marshal failed", "namespace", ev.Namespace, "error", err)
		return
	}
	if err := p.rdb.Publish(ctx, CatalogEventChannel(ev.Namespace), payload).Err(); err != nil {
		// Pub/sub publish errors are non-fatal — the DB state is the truth.
		// Log so an operator can correlate but don't surface to the embed path.
		slog.DebugContext(ctx, "catalog event publish failed", "namespace", ev.Namespace, "error", fmt.Sprintf("%v", err))
	}
}
