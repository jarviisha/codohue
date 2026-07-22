package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/admin/eventbus"
	"github.com/jarviisha/codohue/internal/compute"
)

// batchRunEventsBridge fans batch-run lifecycle messages from Redis pub/sub
// onto the admin event bus. cmd/cron publishes them there, which is the only
// way an admin server in a different process can stream a cron run: an
// in-process observer only ever saw admin-triggered runs, so streaming a
// cron run returned nothing but heartbeats.
//
// Same shape (and failure policy) as catalogEventsBridge: reconnect with a
// small backoff, log and drop unparseable messages rather than break the
// receive loop.
type batchRunEventsBridge struct {
	rdb *goredis.Client
	bus *eventbus.Bus
}

func newBatchRunEventsBridge(rdb *goredis.Client, bus *eventbus.Bus) *batchRunEventsBridge {
	return &batchRunEventsBridge{rdb: rdb, bus: bus}
}

// Run subscribes to the batch-run channel and republishes every message onto
// the admin event bus. Returns when ctx is cancelled.
func (b *batchRunEventsBridge) Run(ctx context.Context) {
	slog.Info("batch run events bridge started", "channel", compute.BatchRunEventChannel)

	const reconnectBackoff = 2 * time.Second
	for {
		if ctx.Err() != nil {
			slog.Info("batch run events bridge stopped")
			return
		}
		if err := b.runOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("batch run events bridge: subscription error, retrying", "error", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(reconnectBackoff):
			}
		}
	}
}

func (b *batchRunEventsBridge) runOnce(ctx context.Context) error {
	pubsub := b.rdb.Subscribe(ctx, compute.BatchRunEventChannel)
	defer func() {
		if err := pubsub.Close(); err != nil {
			slog.Warn("batch run events bridge: pubsub close", "error", err)
		}
	}()
	if _, err := pubsub.Receive(ctx); err != nil {
		return fmt.Errorf("pubsub receive: %w", err)
	}

	ch := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context done: %w", ctx.Err())
		case msg, ok := <-ch:
			if !ok {
				return errors.New("pubsub channel closed")
			}
			b.handle(ctx, msg)
		}
	}
}

// handle decodes one message and republishes it as the matching bus kind.
// The bus kinds are the same vocabulary the per-run and ops SSE streams
// already subscribe to, so no handler changes are needed.
func (b *batchRunEventsBridge) handle(ctx context.Context, msg *goredis.Message) {
	var ev compute.BatchRunEvent
	if err := json.Unmarshal([]byte(msg.Payload), &ev); err != nil {
		slog.Warn("batch run events bridge: malformed payload", "error", err)
		return
	}

	payload := map[string]any{"id": ev.RunID, "namespace": ev.Namespace}
	switch ev.Kind {
	case "started":
		payload["trigger_source"] = ev.TriggerSource
	case "phase_started":
		payload["phase"] = ev.Phase
	case "phase_completed":
		payload["phase"] = ev.Phase
		payload["ok"] = ev.PhaseOK != nil && *ev.PhaseOK
		payload["duration_ms"] = ev.DurationMs
		payload["count1"] = ev.Count1
		payload["count2"] = ev.Count2
		payload["error"] = ev.ErrorMessage
	case "log_line":
		payload = map[string]any{"ts": ev.LogTs, "level": ev.LogLevel, "msg": ev.LogMsg}
	case "completed":
		payload["success"] = ev.Success != nil && *ev.Success
		payload["error_message"] = ev.ErrorMessage
	case "cancelled":
		// id + namespace only.
	default:
		slog.Debug("batch run events bridge: unknown kind", "kind", ev.Kind)
		return
	}

	b.bus.Publish(ctx, eventbus.Event{
		Kind:      "batch_run." + ev.Kind,
		Namespace: ev.Namespace,
		EntityID:  strconv.FormatInt(ev.RunID, 10),
		Payload:   payload,
	})
}
