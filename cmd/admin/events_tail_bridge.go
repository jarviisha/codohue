package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/admin"
	"github.com/jarviisha/codohue/internal/admin/eventbus"
	"github.com/jarviisha/codohue/internal/ingest"
)

// eventsTailBridge fans Redis pub/sub messages published by cmd/api's ingest
// path onto the admin event bus, and feeds the per-namespace rate tracker.
// Lives in cmd/admin (not internal/admin) so the wiring layer absorbs the
// cross-domain ingest + redis imports.
//
// Why pub/sub and not an XREAD tail of `codohue:events`: the HTTP ingest path
// (the primary client transport, and the path admin "inject test event" uses)
// writes straight to the events table and never touches the Redis stream. A
// stream tail would therefore miss most real traffic. Publishing from
// ingest.Service.Process captures every event regardless of transport.
type eventsTailBridge struct {
	rdb     *goredis.Client
	bus     *eventbus.Bus
	tracker *admin.EventRateTracker
}

func newEventsTailBridge(rdb *goredis.Client, bus *eventbus.Bus, tracker *admin.EventRateTracker) *eventsTailBridge {
	return &eventsTailBridge{rdb: rdb, bus: bus, tracker: tracker}
}

// Run subscribes to `codohue:events-tail:*` and republishes each message onto
// the admin bus as kind="events.ingested". Returns when ctx is cancelled.
func (b *eventsTailBridge) Run(ctx context.Context) {
	slog.Info("events tail bridge started", "pattern", ingest.EventTailChannelPattern)

	const reconnectBackoff = 2 * time.Second
	for {
		if ctx.Err() != nil {
			slog.Info("events tail bridge stopped")
			return
		}
		if err := b.runOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("events tail bridge: subscription error, retrying", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(reconnectBackoff):
			}
		}
	}
}

func (b *eventsTailBridge) runOnce(ctx context.Context) error {
	pubsub := b.rdb.PSubscribe(ctx, ingest.EventTailChannelPattern)
	defer func() { _ = pubsub.Close() }()
	if _, err := pubsub.Receive(ctx); err != nil {
		return err
	}

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-ch:
			if !ok {
				return errors.New("pubsub channel closed")
			}
			b.handle(ctx, msg)
		}
	}
}

// handle parses one tail message and republishes it. Bad JSON is logged +
// dropped — bus consumers should never observe a broken message. The bus
// payload mirrors admin.EventSummary so the SPA renders rows without a refetch.
func (b *eventsTailBridge) handle(ctx context.Context, msg *goredis.Message) {
	ns := strings.TrimPrefix(msg.Channel, "codohue:events-tail:")
	if ns == msg.Channel {
		slog.Warn("events tail bridge: unrecognised channel", "channel", msg.Channel)
		return
	}

	var ev ingest.EventTailMessage
	if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
		slog.Warn("events tail bridge: malformed payload", "channel", msg.Channel, "error", err)
		return
	}

	if b.tracker != nil {
		b.tracker.Observe(ns)
	}

	b.bus.Publish(ctx, eventbus.Event{
		Kind:      "events.ingested",
		Namespace: ns,
		EntityID:  strconv.FormatInt(ev.ID, 10),
		Payload: map[string]any{
			"id":          ev.ID,
			"namespace":   ev.Namespace,
			"subject_id":  ev.SubjectID,
			"object_id":   ev.ObjectID,
			"action":      ev.Action,
			"weight":      ev.Weight,
			"occurred_at": ev.OccurredAt.UTC().Format(time.RFC3339),
		},
	})
}
