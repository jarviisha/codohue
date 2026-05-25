package main

import (
	"context"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/admin/eventbus"
)

func TestCatalogBridgeRepublishesGoodPayload(t *testing.T) {
	bus := eventbus.NewBus()
	defer bus.Close()
	ch, cancel := bus.Subscribe(eventbus.Filter{Kinds: []string{"catalog.item_state_changed"}})
	defer cancel()

	bridge := newCatalogEventsBridge(nil, bus) // rdb unused — we drive handle() directly
	bridge.handle(context.Background(), &goredis.Message{
		Channel: "codohue:catalog-events:prod",
		Payload: `{"kind":"item_state_changed","namespace":"prod","item_id":42,"object_id":"sku_42","from":"in_flight","to":"embedded","at":"2025-05-25T10:00:00Z"}`,
	})

	select {
	case ev := <-ch:
		if ev.Namespace != "prod" {
			t.Errorf("Namespace=%q, want prod", ev.Namespace)
		}
		if ev.EntityID != "42" {
			t.Errorf("EntityID=%q, want 42", ev.EntityID)
		}
		payload, ok := ev.Payload.(map[string]any)
		if !ok {
			t.Fatalf("Payload type=%T, want map[string]any", ev.Payload)
		}
		if payload["to"] != "embedded" {
			t.Errorf("payload[to]=%v, want embedded", payload["to"])
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for republished event")
	}
}

func TestCatalogBridgeDropsMalformedPayload(t *testing.T) {
	bus := eventbus.NewBus()
	defer bus.Close()
	ch, cancel := bus.Subscribe(eventbus.Filter{})
	defer cancel()

	bridge := newCatalogEventsBridge(nil, bus)
	bridge.handle(context.Background(), &goredis.Message{
		Channel: "codohue:catalog-events:prod",
		Payload: "{not json",
	})

	select {
	case ev := <-ch:
		t.Fatalf("unexpected event published: %+v", ev)
	case <-time.After(100 * time.Millisecond):
		// expected — bad payload is dropped
	}
}

func TestCatalogBridgeIgnoresUnknownChannel(t *testing.T) {
	bus := eventbus.NewBus()
	defer bus.Close()
	ch, cancel := bus.Subscribe(eventbus.Filter{})
	defer cancel()

	bridge := newCatalogEventsBridge(nil, bus)
	bridge.handle(context.Background(), &goredis.Message{
		Channel: "some:other:channel",
		Payload: `{"kind":"x"}`,
	})

	select {
	case ev := <-ch:
		t.Fatalf("unexpected event: %+v", ev)
	case <-time.After(100 * time.Millisecond):
	}
}
