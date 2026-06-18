package admin

import (
	"context"
	"testing"
	"time"
)

// fakeEventRate is a stub eventRateReader for service tests.
type fakeEventRate struct {
	rates map[string]float64
}

func (f fakeEventRate) RatePerSec(ns string, _ time.Duration) float64 { return f.rates[ns] }
func (f fakeEventRate) RatesPerSec(_ time.Duration) map[string]float64 {
	return f.rates
}

func TestGetEventsSummary_RatesAndSorting(t *testing.T) {
	repo := &fakeRepo{
		eventsSummaryTotal:    800,
		eventsSummaryByAction: map[string]int{"view": 600, "like": 200},
		eventsSummarySeries:   []EventsSummaryBucket{{Ts: "2026-06-01T00:00:00Z", Count: 13}},
	}
	svc := newTestService(repo, "", "")

	out, err := svc.GetEventsSummary(context.Background(), "prod", time.Minute, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.WindowSeconds != 60 || out.BucketSeconds != 1 {
		t.Errorf("window/bucket: got %d/%d", out.WindowSeconds, out.BucketSeconds)
	}
	if out.Total != 800 {
		t.Errorf("total: got %d, want 800", out.Total)
	}
	wantRate := 800.0 / 60.0
	if out.RatePerSecond != wantRate {
		t.Errorf("rate: got %v, want %v", out.RatePerSecond, wantRate)
	}
	// Highest-volume action first.
	if len(out.ByAction) != 2 || out.ByAction[0].Action != "view" {
		t.Fatalf("by_action order: %+v", out.ByAction)
	}
	if out.ByAction[0].Rate != 600.0/60.0 {
		t.Errorf("view rate: got %v", out.ByAction[0].Rate)
	}
	if len(out.Series) != 1 {
		t.Errorf("series: got %d entries", len(out.Series))
	}
}

func TestGetMetricsSummary_IngestRatesFromTracker(t *testing.T) {
	svc := newTestService(&fakeRepo{}, "", "")
	svc.SetEventRateTracker(fakeEventRate{rates: map[string]float64{"prod": 14.2}})

	out, err := svc.GetMetricsSummary(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Ingest.EventsPerSec1m["prod"] != 14.2 {
		t.Errorf("events_per_sec_1m: got %v", out.Ingest.EventsPerSec1m["prod"])
	}
	if out.Cron.BatchLagSeconds != 0 {
		t.Errorf("expected 0 cron lag with no runs, got %v", out.Cron.BatchLagSeconds)
	}
	if out.GeneratedAt == "" {
		t.Error("generated_at should be set")
	}
}

func TestGetMetricsSummary_NilTrackerIsZero(t *testing.T) {
	svc := newTestService(&fakeRepo{}, "", "")
	out, err := svc.GetMetricsSummary(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Ingest.EventsPerSec1m) != 0 {
		t.Errorf("expected empty ingest map, got %+v", out.Ingest.EventsPerSec1m)
	}
}
