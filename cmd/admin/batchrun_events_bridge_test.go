package main

import (
	"context"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/admin/eventbus"
)

// receive waits briefly for one event on ch, failing the test on timeout.
func receive(t *testing.T, ch <-chan eventbus.Event) eventbus.Event {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for a bus event")
		return eventbus.Event{}
	}
}

// A cron run's stream used to emit nothing but heartbeats, because the only
// observer lived inside cmd/admin. The bridge is what carries those runs
// across the process boundary.
func TestBatchRunBridgeRepublishesPhaseCompleted(t *testing.T) {
	bus := eventbus.NewBus()
	defer bus.Close()
	ch, cancel := bus.Subscribe(eventbus.Filter{
		EntityID: "7",
		Kinds:    []string{"batch_run.phase_completed"},
	})
	defer cancel()

	bridge := newBatchRunEventsBridge(nil, bus) // rdb unused — handle() driven directly
	bridge.handle(context.Background(), &goredis.Message{
		Channel: "codohue:batchrun-events",
		Payload: `{"kind":"phase_completed","run_id":7,"namespace":"prod","phase":2,"phase_ok":true,"duration_ms":15,"count1":3,"count2":4}`,
	})

	ev := receive(t, ch)
	if ev.Namespace != "prod" || ev.EntityID != "7" {
		t.Fatalf("identity: ns=%q entity=%q", ev.Namespace, ev.EntityID)
	}
	payload, ok := ev.Payload.(map[string]any)
	if !ok {
		t.Fatalf("payload type %T", ev.Payload)
	}
	if payload["phase"] != 2 || payload["ok"] != true || payload["duration_ms"] != 15 {
		t.Fatalf("payload: %+v", payload)
	}
}

func TestBatchRunBridgeRepublishesTerminalKinds(t *testing.T) {
	for _, tc := range []struct {
		payload  string
		wantKind string
	}{
		{`{"kind":"completed","run_id":9,"namespace":"prod","success":true}`, "batch_run.completed"},
		{`{"kind":"cancelled","run_id":9,"namespace":"prod"}`, "batch_run.cancelled"},
		{`{"kind":"started","run_id":9,"namespace":"prod","trigger_source":"cron"}`, "batch_run.started"},
	} {
		bus := eventbus.NewBus()
		ch, cancel := bus.Subscribe(eventbus.Filter{Kinds: []string{tc.wantKind}})

		newBatchRunEventsBridge(nil, bus).handle(context.Background(), &goredis.Message{Payload: tc.payload})

		ev := receive(t, ch)
		if ev.Kind != tc.wantKind {
			t.Errorf("kind: got %q, want %q", ev.Kind, tc.wantKind)
		}
		cancel()
		bus.Close()
	}
}

func TestBatchRunBridgeDropsMalformedAndUnknown(t *testing.T) {
	bus := eventbus.NewBus()
	defer bus.Close()
	ch, cancel := bus.Subscribe(eventbus.Filter{})
	defer cancel()

	bridge := newBatchRunEventsBridge(nil, bus)
	bridge.handle(context.Background(), &goredis.Message{Payload: `not json`})
	bridge.handle(context.Background(), &goredis.Message{Payload: `{"kind":"who_knows","run_id":1}`})

	select {
	case ev := <-ch:
		t.Fatalf("bad payloads must be dropped, got %+v", ev)
	case <-time.After(100 * time.Millisecond):
	}
}
