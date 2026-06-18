package admin

import (
	"testing"
	"time"
)

func TestEventRateTracker_RatePerSec(t *testing.T) {
	tr := NewEventRateTracker()
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

	// Slots are created lazily on first Observe, so prime with one event before
	// the baseline sample. Then 600 events arrive over the next 60s: cumulative
	// goes 1 → 601, a delta of 600 over 60s = 10 events/sec.
	tr.Observe("prod")
	tr.Sample(base)
	for i := 0; i < 600; i++ {
		tr.Observe("prod")
	}
	tr.Sample(base.Add(60 * time.Second))

	if rate := tr.RatePerSec("prod", time.Minute); rate != 10 {
		t.Errorf("rate: got %v, want 10 (600 events / 60s)", rate)
	}
}

func TestEventRateTracker_UnknownNamespaceIsZero(t *testing.T) {
	tr := NewEventRateTracker()
	if got := tr.RatePerSec("nope", time.Minute); got != 0 {
		t.Errorf("unknown ns rate: got %v, want 0", got)
	}
}

func TestEventRateTracker_RatesPerSecCoversAllNamespaces(t *testing.T) {
	tr := NewEventRateTracker()
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	tr.Observe("a")
	tr.Observe("b")
	tr.Sample(base)
	tr.Observe("a")
	tr.Sample(base.Add(10 * time.Second))

	rates := tr.RatesPerSec(time.Minute)
	if _, ok := rates["a"]; !ok {
		t.Error("expected namespace a in rates map")
	}
	if _, ok := rates["b"]; !ok {
		t.Error("expected namespace b in rates map")
	}
	if ns := tr.Namespaces(); len(ns) != 2 || ns[0] != "a" || ns[1] != "b" {
		t.Errorf("namespaces: got %v, want [a b]", ns)
	}
}
