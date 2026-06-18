package main

import (
	"context"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/admin"
	"github.com/jarviisha/codohue/internal/admin/eventbus"
)

func TestEventsTailBridge_HandleRepublishesAndTracks(t *testing.T) {
	bus := eventbus.NewBus()
	defer bus.Close()
	tracker := admin.NewEventRateTracker()
	bridge := newEventsTailBridge(nil, bus, tracker)

	events, cancel := bus.Subscribe(eventbus.Filter{Namespace: "prod", Kinds: []string{"events.ingested"}})
	defer cancel()

	bridge.handle(context.Background(), &goredis.Message{
		Channel: "codohue:events-tail:prod",
		Payload: `{"id":7,"namespace":"prod","subject_id":"u1","object_id":"o1","action":"LIKE","weight":5,"occurred_at":"2026-06-01T00:00:00Z"}`,
	})

	select {
	case e := <-events:
		if e.Kind != "events.ingested" || e.Namespace != "prod" {
			t.Fatalf("unexpected event: %+v", e)
		}
		payload, ok := e.Payload.(map[string]any)
		if !ok {
			t.Fatalf("payload type: %T", e.Payload)
		}
		if payload["subject_id"] != "u1" || payload["action"] != "LIKE" {
			t.Errorf("payload mismatch: %+v", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("expected republished event on bus")
	}

	if ns := tracker.Namespaces(); len(ns) != 1 || ns[0] != "prod" {
		t.Errorf("tracker did not observe namespace: %v", ns)
	}
}

func TestEventsTailBridge_DropsBadPayload(t *testing.T) {
	bus := eventbus.NewBus()
	defer bus.Close()
	bridge := newEventsTailBridge(nil, bus, admin.NewEventRateTracker())

	events, cancel := bus.Subscribe(eventbus.Filter{})
	defer cancel()

	bridge.handle(context.Background(), &goredis.Message{
		Channel: "codohue:events-tail:prod",
		Payload: `{not json`,
	})

	select {
	case e := <-events:
		t.Fatalf("expected no event for bad payload, got %+v", e)
	case <-time.After(50 * time.Millisecond):
		// no event — correct
	}
}
