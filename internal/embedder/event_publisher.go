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

// CatalogBacklogSnapshotPayload is the structured backlog body inside a
// backlog_snapshot event. Same shape as the admin CatalogBacklog wire type
// so the SPA can re-render tiles without a refetch.
type CatalogBacklogSnapshotPayload struct {
	Pending    int `json:"pending"`
	InFlight   int `json:"in_flight"`
	Failed     int `json:"failed"`
	DeadLetter int `json:"dead_letter"`
	StreamLen  int `json:"stream_len"`
}

// CatalogBacklogSnapshotEvent fires when the sampler writes a new sample
// (counts changed or ForceWriteAfter elapsed). Drives live tile updates on
// the Catalog status page between polling refetches.
type CatalogBacklogSnapshotEvent struct {
	Kind      string                        `json:"kind"` // always "backlog_snapshot"
	Namespace string                        `json:"namespace"`
	Backlog   CatalogBacklogSnapshotPayload `json:"backlog"`
	At        time.Time                     `json:"at"`
}

// CatalogDeadLetterGrewEvent fires when dead_letter rose since the previous
// sample. Drives the global ops toast + Status-page alert so operators see
// problems land in dead-letter without staring at the catalog page.
type CatalogDeadLetterGrewEvent struct {
	Kind          string    `json:"kind"` // always "dead_letter_grew"
	Namespace     string    `json:"namespace"`
	PreviousCount int       `json:"previous_count"`
	NewCount      int       `json:"new_count"`
	Delta         int       `json:"delta"`
	At            time.Time `json:"at"`
}

// CatalogReembedProgressEvent fires once per ReembedWatcher tick for every
// open re-embed run. Drives the ReembedOverlay progress bar so operators
// see how far along a re-embed is without leaving the page they're on.
//
// `Processed` counts catalog_items in state='embedded' at the namespace's
// active strategy_version; `Total` is processed + still-stale items (the
// rows the watcher waits on before closing the run).
type CatalogReembedProgressEvent struct {
	Kind       string    `json:"kind"` // always "reembed_progress"
	Namespace  string    `json:"namespace"`
	BatchRunID int64     `json:"batch_run_id"`
	Processed  int       `json:"processed"`
	Total      int       `json:"total"`
	At         time.Time `json:"at"`
}

// CatalogEventPublisher publishes catalog events to a transport the admin
// plane can subscribe to. Production uses Redis pub/sub; tests inject an
// in-memory fake.
type CatalogEventPublisher interface {
	PublishItemStateChanged(ctx context.Context, ev CatalogItemStateChangedEvent)
	PublishBacklogSnapshot(ctx context.Context, ev CatalogBacklogSnapshotEvent)
	PublishDeadLetterGrew(ctx context.Context, ev CatalogDeadLetterGrewEvent)
	PublishReembedProgress(ctx context.Context, ev CatalogReembedProgressEvent)
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

// PublishItemStateChanged publishes an item_state_changed event onto the
// namespace's Redis pub/sub channel, defaulting the kind and timestamp.
func (p *redisPubsubPublisher) PublishItemStateChanged(ctx context.Context, ev CatalogItemStateChangedEvent) {
	if ev.Kind == "" {
		ev.Kind = "item_state_changed"
	}
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	p.publish(ctx, ev.Namespace, ev)
}

// PublishBacklogSnapshot publishes a backlog_snapshot event onto the
// namespace's Redis pub/sub channel, defaulting the kind and timestamp.
func (p *redisPubsubPublisher) PublishBacklogSnapshot(ctx context.Context, ev CatalogBacklogSnapshotEvent) {
	if ev.Kind == "" {
		ev.Kind = "backlog_snapshot"
	}
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	p.publish(ctx, ev.Namespace, ev)
}

// PublishDeadLetterGrew publishes a dead_letter_grew event onto the
// namespace's Redis pub/sub channel, defaulting the kind and timestamp.
func (p *redisPubsubPublisher) PublishDeadLetterGrew(ctx context.Context, ev CatalogDeadLetterGrewEvent) {
	if ev.Kind == "" {
		ev.Kind = "dead_letter_grew"
	}
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	p.publish(ctx, ev.Namespace, ev)
}

// PublishReembedProgress publishes a reembed_progress event onto the
// namespace's Redis pub/sub channel, defaulting the kind and timestamp.
func (p *redisPubsubPublisher) PublishReembedProgress(ctx context.Context, ev CatalogReembedProgressEvent) {
	if ev.Kind == "" {
		ev.Kind = "reembed_progress"
	}
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	p.publish(ctx, ev.Namespace, ev)
}

// publish is the shared marshal-then-PUBLISH helper. Errors are logged but
// never surfaced — pub/sub is best-effort observability, not a critical path.
func (p *redisPubsubPublisher) publish(ctx context.Context, namespace string, ev any) {
	payload, err := json.Marshal(ev)
	if err != nil {
		slog.WarnContext(ctx, "catalog event marshal failed", "namespace", namespace, "error", err)
		return
	}
	if err := p.rdb.Publish(ctx, CatalogEventChannel(namespace), payload).Err(); err != nil {
		slog.DebugContext(ctx, "catalog event publish failed", "namespace", namespace, "error", fmt.Sprintf("%v", err))
	}
}
