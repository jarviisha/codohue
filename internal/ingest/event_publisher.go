package ingest

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// EventTailMessage is the JSON payload published to Redis pub/sub once per
// successfully-ingested event. cmd/admin's events-tail bridge subscribes to
// `codohue:events-tail:*` and republishes onto the in-process event bus so the
// live event tail (SSE) fans it out to operators.
//
// Both ingest transports — the HTTP endpoint and the Redis Streams worker —
// funnel through Service.Process, so publishing from there captures every
// event regardless of how it arrived (including admin-injected test events,
// which proxy to the HTTP endpoint). A stream-tail of `codohue:events` would
// miss the HTTP path entirely, which is why this uses a dedicated fan-out.
type EventTailMessage struct {
	ID         int64     `json:"id"`
	Namespace  string    `json:"namespace"`
	SubjectID  string    `json:"subject_id"`
	ObjectID   string    `json:"object_id"`
	Action     string    `json:"action"`
	Weight     float64   `json:"weight"`
	OccurredAt time.Time `json:"occurred_at"`
}

// EventTailChannel is the per-namespace Redis pub/sub channel the tail flows
// over. Centralised so the cmd/api publisher and the cmd/admin subscriber
// agree on the wire — keep them in lockstep.
func EventTailChannel(namespace string) string {
	return "codohue:events-tail:" + namespace
}

// EventTailChannelPattern matches every namespace's tail channel. The admin
// bridge psubscribes to this pattern so it need not track enabled namespaces.
const EventTailChannelPattern = "codohue:events-tail:*"

// EventTailPublisher fans one message out per ingested event. Implementations
// MUST be non-blocking: the tail is best-effort observability and must never
// add latency to — or fail — the ingest hot path.
type EventTailPublisher interface {
	Publish(msg EventTailMessage)
}

// RedisEventTailPublisher publishes tail messages to Redis pub/sub off the hot
// path. Publish enqueues onto a bounded buffer and returns immediately; a
// single background goroutine (Run) drains the buffer and issues the actual
// PUBLISH. When the buffer is full the oldest-pending message is dropped — at
// 1k events/s a momentarily-stalled Redis must never back-pressure ingest.
type RedisEventTailPublisher struct {
	rdb     *redis.Client
	ch      chan EventTailMessage
	dropped func() // optional hook fired once per dropped message (metrics/tests)
}

// NewRedisEventTailPublisher wraps a Redis client. buffer caps the in-flight
// queue (defaults to 4096 when <= 0). Call Run in a goroutine to start
// draining.
func NewRedisEventTailPublisher(rdb *redis.Client, buffer int) *RedisEventTailPublisher {
	if buffer <= 0 {
		buffer = 4096
	}
	return &RedisEventTailPublisher{
		rdb: rdb,
		ch:  make(chan EventTailMessage, buffer),
	}
}

// Publish enqueues msg for asynchronous delivery. Never blocks: when the buffer
// is full it drops the new message and fires the drop hook (if set).
func (p *RedisEventTailPublisher) Publish(msg EventTailMessage) {
	select {
	case p.ch <- msg:
	default:
		if p.dropped != nil {
			p.dropped()
		}
	}
}

// Run drains the buffer and PUBLISHes each message until ctx is cancelled.
// Publish failures are logged at debug and dropped — pub/sub is best-effort.
func (p *RedisEventTailPublisher) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-p.ch:
			payload, err := json.Marshal(msg)
			if err != nil {
				slog.WarnContext(ctx, "event tail marshal failed", "namespace", msg.Namespace, "error", err)
				continue
			}
			if err := p.rdb.Publish(ctx, EventTailChannel(msg.Namespace), payload).Err(); err != nil {
				slog.DebugContext(ctx, "event tail publish failed", "namespace", msg.Namespace, "error", err)
			}
		}
	}
}
