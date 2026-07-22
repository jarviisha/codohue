package admin

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestGetOverviewBasicShape(t *testing.T) {
	now := time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC)
	repo := &fakeRepo{
		namespaces: []NamespaceConfig{{Namespace: "prod"}, {Namespace: "staging"}},
		lastBatchRuns: map[string]BatchRunLog{
			"prod": {ID: 1, Namespace: "prod", StartedAt: now.Add(-5 * time.Minute), Success: true, TriggerSource: "cron", Phase1OK: ptrB(true)},
		},
		recentEventCounts: map[string]int{"prod": 5000},
	}
	svc := newTestService(repo, "http://api", "k")
	svc.SetNowFn(func() time.Time { return now })

	out, err := svc.GetOverview(context.Background())
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(out.Namespaces) != 2 {
		t.Fatalf("namespaces=%d, want 2", len(out.Namespaces))
	}

	// Prod has a successful cron run + events → active.
	if out.Namespaces[0].Namespace != "prod" || out.Namespaces[0].Status != NSStatusActive {
		t.Errorf("prod row=%+v", out.Namespaces[0])
	}
	// Staging has no last run → cold.
	if out.Namespaces[1].Status != NSStatusCold {
		t.Errorf("staging status=%q, want cold", out.Namespaces[1].Status)
	}
	if !out.CronHeartbeat.OK || out.CronHeartbeat.LagSeconds < 290 || out.CronHeartbeat.LagSeconds > 310 {
		t.Errorf("cron heartbeat=%+v, want ok with lag ~300s", out.CronHeartbeat)
	}
}

func TestGetOverviewAlertsRunFailed(t *testing.T) {
	now := time.Now()
	errMsg := "something exploded"
	repo := &fakeRepo{
		namespaces: []NamespaceConfig{{Namespace: "prod"}},
		lastBatchRuns: map[string]BatchRunLog{
			"prod": {ID: 1, Namespace: "prod", StartedAt: now.Add(-2 * time.Minute), Success: false, ErrorMessage: &errMsg, TriggerSource: "cron"},
		},
	}
	svc := newTestService(repo, "http://api", "k")
	svc.SetNowFn(func() time.Time { return now })

	out, err := svc.GetOverview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Alerts) == 0 {
		t.Fatal("expected at least one alert")
	}
	if out.Alerts[0].Kind != "run_failed" {
		t.Errorf("alert kind=%q, want run_failed", out.Alerts[0].Kind)
	}
}

func TestGetOverviewSkipsCancelledRunInAlerts(t *testing.T) {
	now := time.Now()
	cancelled := "operator_cancelled"
	repo := &fakeRepo{
		namespaces: []NamespaceConfig{{Namespace: "prod"}},
		lastBatchRuns: map[string]BatchRunLog{
			"prod": {ID: 1, Namespace: "prod", StartedAt: now.Add(-2 * time.Minute), Success: false, ErrorMessage: &cancelled, TriggerSource: "cron"},
		},
	}
	svc := newTestService(repo, "http://api", "k")
	svc.SetNowFn(func() time.Time { return now })

	out, err := svc.GetOverview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range out.Alerts {
		if a.Kind == "run_failed" {
			t.Fatal("operator-cancelled rows must not produce run_failed alerts")
		}
	}
}

func TestGetNamespaceDashboardNotFoundReturnsNil(t *testing.T) {
	repo := &fakeRepo{namespace: nil}
	svc := newTestService(repo, "http://api", "k")

	out, err := svc.GetNamespaceDashboard(context.Background(), "missing")
	if err != nil {
		t.Fatal(err)
	}
	if out != nil {
		t.Fatalf("got %+v, want nil for 404", out)
	}
}

func TestGetNamespaceDashboardAggregates(t *testing.T) {
	now := time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC)
	repo := &fakeRepo{
		namespace: &NamespaceConfig{Namespace: "prod", Lambda: 0.05},
		batchRuns: []BatchRunLog{
			{ID: 1, Namespace: "prod", TriggerSource: "cron", Phase1OK: ptrB(true)},
			{ID: 2, Namespace: "prod", TriggerSource: "cron", Phase1OK: ptrB(false)},
		},
		recentEventCounts: map[string]int{"prod": 100, "other": 999},
	}
	svc := newTestService(repo, "http://api", "k")
	svc.SetNowFn(func() time.Time { return now })

	out, err := svc.GetNamespaceDashboard(context.Background(), "prod")
	if err != nil {
		t.Fatal(err)
	}
	if out.Namespace != "prod" {
		t.Errorf("namespace=%q", out.Namespace)
	}
	if len(out.LastRuns) != 2 {
		t.Errorf("last_runs=%d, want 2", len(out.LastRuns))
	}
	if out.Events24h != 100 {
		t.Errorf("events24h=%d, want 100", out.Events24h)
	}
}

func TestGetNamespaceDashboard_ReportsAuthorCoverage(t *testing.T) {
	repo := &fakeRepo{
		namespace:        &NamespaceConfig{Namespace: "prod"},
		authorAttributed: 7,
		authorTotal:      8,
	}
	svc := newTestService(repo, "http://api", "k")

	out, err := svc.GetNamespaceDashboard(context.Background(), "prod")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.AuthorCoverage.Attributed != 7 || out.AuthorCoverage.Total != 8 {
		t.Errorf("author coverage = %+v, want 7/8", out.AuthorCoverage)
	}
}

// Coverage is an advisory line on one form field — a failed count must not
// take the whole dashboard down with it.
func TestGetNamespaceDashboard_AuthorCoverageErrorDegrades(t *testing.T) {
	repo := &fakeRepo{
		namespace:         &NamespaceConfig{Namespace: "prod"},
		authorCoverageErr: errors.New("db down"),
	}
	svc := newTestService(repo, "http://api", "k")

	out, err := svc.GetNamespaceDashboard(context.Background(), "prod")
	if err != nil {
		t.Fatalf("dashboard must still render, got error: %v", err)
	}
	if out.AuthorCoverage.Attributed != 0 || out.AuthorCoverage.Total != 0 {
		t.Errorf("expected zeroed coverage on error, got %+v", out.AuthorCoverage)
	}
}

// The overview used to hardcode "embedder OK", claiming a healthy worker even
// when the process was down. With no Redis (or no heartbeat key) we cannot
// know it is alive, and must not say it is.
func TestGetOverview_EmbedderHeartbeatNotFakedWhenUnknown(t *testing.T) {
	svc := newTestService(&fakeRepo{}, "", "")

	resp, err := svc.GetOverview(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.EmbedderHeartbeat.OK {
		t.Fatal("embedder heartbeat must not report OK without a liveness signal")
	}
}
