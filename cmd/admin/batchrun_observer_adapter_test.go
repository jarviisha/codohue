package main

import (
	"context"
	"testing"
	"time"

	"github.com/jarviisha/codohue/internal/admin/eventbus"
	"github.com/jarviisha/codohue/internal/compute"
	"github.com/jarviisha/codohue/internal/core/batchrun"
)

// drain pulls up to n events off the channel with a per-event timeout.
func drain(t *testing.T, ch <-chan eventbus.Event, n int) []eventbus.Event {
	t.Helper()
	out := make([]eventbus.Event, 0, n)
	for i := 0; i < n; i++ {
		select {
		case e := <-ch:
			out = append(out, e)
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for event %d/%d (got %d)", i+1, n, len(out))
		}
	}
	return out
}

func TestObserverAdapterPublishesEveryCallback(t *testing.T) {
	bus := eventbus.NewBus()
	defer bus.Close()

	ch, cancel := bus.Subscribe(eventbus.Filter{EntityID: "42"})
	defer cancel()

	o := newBatchRunObserverAdapter(bus)
	o.OnRunStarted(42, "prod", batchrun.TriggerCron)
	o.OnPhaseStarted(42, "prod", 1)
	o.OnPhaseCompleted(42, "prod", 1, compute.PhaseResult{OK: true, DurationMs: 100, Count1: 5, Count2: 7})
	o.OnLogLine(42, "prod", compute.LogEntry{Ts: "t", Level: "info", Msg: "hi"})
	o.OnRunCompleted(42, "prod", true, "")

	events := drain(t, ch, 5)
	wantKinds := []string{
		"batch_run.started",
		"batch_run.phase_started",
		"batch_run.phase_completed",
		"batch_run.log_line",
		"batch_run.completed",
	}
	for i, want := range wantKinds {
		if events[i].Kind != want {
			t.Errorf("event[%d].Kind=%q, want %q", i, events[i].Kind, want)
		}
		if events[i].Namespace != "prod" {
			t.Errorf("event[%d].Namespace=%q, want prod", i, events[i].Namespace)
		}
		if events[i].EntityID != "42" {
			t.Errorf("event[%d].EntityID=%q, want 42", i, events[i].EntityID)
		}
	}
}

func TestObserverAdapterCancelledPublishesCancelledKind(t *testing.T) {
	bus := eventbus.NewBus()
	defer bus.Close()

	ch, cancel := bus.Subscribe(eventbus.Filter{Kinds: []string{"batch_run.cancelled"}})
	defer cancel()

	o := newBatchRunObserverAdapter(bus)
	o.OnRunCancelled(99, "staging")

	select {
	case e := <-ch:
		if e.Kind != "batch_run.cancelled" {
			t.Fatalf("Kind=%q, want batch_run.cancelled", e.Kind)
		}
		if e.Namespace != "staging" {
			t.Fatalf("Namespace=%q, want staging", e.Namespace)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestObserverAdapterFilterByNamespaceIsolatesEvents(t *testing.T) {
	bus := eventbus.NewBus()
	defer bus.Close()

	prodCh, cancelProd := bus.Subscribe(eventbus.Filter{Namespace: "prod"})
	defer cancelProd()
	stagingCh, cancelStaging := bus.Subscribe(eventbus.Filter{Namespace: "staging"})
	defer cancelStaging()

	o := newBatchRunObserverAdapter(bus)
	o.OnRunStarted(1, "prod", batchrun.TriggerCron)

	select {
	case e := <-prodCh:
		if e.Namespace != "prod" {
			t.Fatalf("prod sub got namespace %q", e.Namespace)
		}
	case <-time.After(time.Second):
		t.Fatal("prod sub timed out")
	}
	select {
	case <-stagingCh:
		t.Fatal("staging sub should not have received prod event")
	case <-time.After(50 * time.Millisecond):
	}
}

// Compile-time sanity: adapter satisfies compute.BatchRunObserver.
var _ compute.BatchRunObserver = (*batchRunObserverAdapter)(nil)

// Unused context import guard for the build (keeps imports minimal as more
// tests get added).
var _ = context.Background
