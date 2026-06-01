package admin

import (
	"context"
	"testing"
	"time"

	"github.com/jarviisha/codohue/internal/admin/eventbus"
)

// recordingSender captures Send calls on a channel so tests can synchronise.
type recordingSender struct {
	sends chan sentFrame
}

type sentFrame struct {
	event string
	data  any
}

func newRecordingSender() *recordingSender {
	return &recordingSender{sends: make(chan sentFrame, 32)}
}

func (s *recordingSender) Send(event string, data any) error {
	s.sends <- sentFrame{event, data}
	return nil
}

func (s *recordingSender) Ping() error { return nil }

func eventIngested(ns, subject, action string) eventbus.Event {
	return eventbus.Event{
		Kind:      "events.ingested",
		Namespace: ns,
		Payload: map[string]any{
			"subject_id": subject,
			"action":     action,
		},
	}
}

func TestStreamEvents_ForwardsMatching(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	src := make(chan eventbus.Event, 4)
	sender := newRecordingSender()

	go streamEvents(ctx, sender, src, "", "")

	src <- eventIngested("prod", "u1", "view")

	select {
	case f := <-sender.sends:
		if f.event != "event" {
			t.Fatalf("event name: got %q, want event", f.event)
		}
	case <-time.After(time.Second):
		t.Fatal("expected event to be forwarded")
	}
}

func TestStreamEvents_AppliesFilters(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	src := make(chan eventbus.Event, 4)
	sender := newRecordingSender()

	go streamEvents(ctx, sender, src, "like", "")

	src <- eventIngested("prod", "u1", "view") // filtered out
	src <- eventIngested("prod", "u2", "like") // kept

	select {
	case f := <-sender.sends:
		m := f.data.(map[string]any)
		if m["action"] != "like" {
			t.Fatalf("expected only the like event, got %+v", m)
		}
	case <-time.After(time.Second):
		t.Fatal("expected the matching event to be forwarded")
	}
}

func TestMatchEventFilter(t *testing.T) {
	e := eventIngested("prod", "u1", "view")
	cases := []struct {
		action, subject string
		want            bool
	}{
		{"", "", true},
		{"view", "", true},
		{"like", "", false},
		{"", "u1", true},
		{"", "u2", false},
		{"view", "u1", true},
		{"view", "u2", false},
	}
	for _, c := range cases {
		if got := matchEventFilter(e, c.action, c.subject); got != c.want {
			t.Errorf("matchEventFilter(action=%q,subject=%q): got %v, want %v", c.action, c.subject, got, c.want)
		}
	}
	// Non-map payload never matches a non-empty filter.
	if matchEventFilter(eventbus.Event{Payload: "nope"}, "view", "") {
		t.Error("non-map payload should not match a filter")
	}
}
