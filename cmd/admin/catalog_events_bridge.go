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

	"github.com/jarviisha/codohue/internal/admin/eventbus"
	"github.com/jarviisha/codohue/internal/embedder"
)

// catalogEventsBridge fans Redis pub/sub messages from cmd/embedder onto the
// admin event bus. Lives in cmd/admin (not internal/admin) so the wiring
// layer absorbs the cross-domain Redis import without making internal/admin
// depend on the concrete redis client.
//
// Lifecycle: Run blocks on PSubscribe(catalog-events:*) until ctx cancels.
// Reconnect on connection loss is delegated to go-redis (Receive surfaces
// the error; the bridge logs and retries with a small backoff). Messages
// that fail to parse are logged + dropped — we never break the pub/sub
// receiver loop on a single bad payload.
type catalogEventsBridge struct {
	rdb *goredis.Client
	bus *eventbus.Bus
}

func newCatalogEventsBridge(rdb *goredis.Client, bus *eventbus.Bus) *catalogEventsBridge {
	return &catalogEventsBridge{rdb: rdb, bus: bus}
}

// Run subscribes to `codohue:catalog-events:*` on Redis and republishes each
// message onto the admin event bus as kind="catalog.item_state_changed".
// Returns when ctx is cancelled.
func (b *catalogEventsBridge) Run(ctx context.Context) {
	slog.Info("catalog events bridge started", "pattern", embedder.CatalogEventChannelPattern)

	const reconnectBackoff = 2 * time.Second
	for {
		if ctx.Err() != nil {
			slog.Info("catalog events bridge stopped")
			return
		}
		if err := b.runOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("catalog events bridge: subscription error, retrying", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(reconnectBackoff):
			}
		}
	}
}

func (b *catalogEventsBridge) runOnce(ctx context.Context) error {
	pubsub := b.rdb.PSubscribe(ctx, embedder.CatalogEventChannelPattern)
	defer func() {
		_ = pubsub.Close()
	}()
	// Receive() flushes the SUBSCRIBE handshake; surface errors here so the
	// outer retry loop can back off rather than spin tight on a closed conn.
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

// handle parses one pub/sub message and republishes it onto the admin bus.
// Bad payloads are logged + dropped.
func (b *catalogEventsBridge) handle(ctx context.Context, msg *goredis.Message) {
	ns := strings.TrimPrefix(msg.Channel, "codohue:catalog-events:")
	if ns == msg.Channel {
		// PSubscribe pattern guarantees the prefix; if it isn't here, the
		// admin set up its subscription wrong.
		slog.Warn("catalog events bridge: unrecognised channel", "channel", msg.Channel)
		return
	}

	var ev embedder.CatalogItemStateChangedEvent
	if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
		slog.Warn("catalog events bridge: malformed payload", "channel", msg.Channel, "error", err)
		return
	}

	// Republish on the in-process bus. Kind prefix `catalog.` keeps it
	// distinct from `batch_run.*` events; SSE filter subscribes by kind.
	b.bus.Publish(ctx, eventbus.Event{
		Kind:      "catalog.item_state_changed",
		Namespace: ns,
		EntityID:  strconv.FormatInt(ev.ItemID, 10),
		Payload: map[string]any{
			"namespace": ev.Namespace,
			"item_id":   ev.ItemID,
			"object_id": ev.ObjectID,
			"from":      ev.From,
			"to":        ev.To,
			"at":        ev.At,
		},
	})
}
