package admin

import (
	"context"
	"fmt"
	"time"

	"github.com/jarviisha/codohue/internal/core/batchrun"
)

// GetOverview returns the full payload that drives the Fleet overview page.
// Health, cron heartbeat, per-namespace summary, and alert rules are evaluated
// in one call so the SPA does not have to fan out.
//
// Embedder heartbeat and per-namespace ingest rate are stubbed in Phase 1 —
// embedder heartbeat lands when the embedder exposes its liveness signal,
// per-namespace ingest rate lands once Prometheus rolling counters are wired
// in Phase 3.
func (s *Service) GetOverview(ctx context.Context) (*OverviewResponse, error) {
	now := s.nowFn()

	health, _, err := s.GetHealth(ctx)
	if err != nil || health == nil {
		health = &HealthResponse{Status: "unknown"}
	}

	namespaces, err := s.repo.ListNamespaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}
	lastRuns, err := s.repo.GetLastBatchRunPerNamespace(ctx)
	if err != nil {
		return nil, fmt.Errorf("get last runs: %w", err)
	}
	eventCounts, err := s.repo.GetRecentEventCounts(ctx, 24)
	if err != nil {
		return nil, fmt.Errorf("get event counts: %w", err)
	}

	cron := deriveCronHeartbeat(lastRuns, now)
	alerts := buildAlerts(namespaces, lastRuns, now)

	nsOut := make([]NamespaceOverview, 0, len(namespaces))
	for _, ns := range namespaces {
		row := NamespaceOverview{
			Namespace:       ns.Namespace,
			Status:          deriveNamespaceStatus(lastRuns, eventCounts, ns.Namespace),
			Events24h:       eventCounts[ns.Namespace],
			EventsPerMinNow: s.eventsPerMin(ns.Namespace),
			Catalog: NamespaceOverviewCatalog{
				Enabled: ns.CatalogEnabled,
				// Pending / DeadLetter populated by the catalog stream once
				// Phase 2 lands; Phase 1 leaves zeros so the shape stays stable.
			},
		}
		if run, ok := lastRuns[ns.Namespace]; ok {
			r := run
			row.LastRun = &NamespaceOverviewRun{
				ID:          r.ID,
				StartedAt:   r.StartedAt,
				Success:     r.Success,
				PhaseStatus: [3]*string{derivePhaseStatus(r.Phase1OK), derivePhaseStatus(r.Phase2OK), derivePhaseStatus(r.Phase3OK)},
			}
		}
		nsOut = append(nsOut, row)
	}

	return &OverviewResponse{
		GeneratedAt:   now.UTC(),
		Health:        *health,
		CronHeartbeat: cron,
		EmbedderHeartbeat: EmbedderHeartbeat{
			// Phase 1 stub: no liveness signal wired yet. Default OK so the
			// UI doesn't render a spurious "embedder silent" alert before
			// the actual heartbeat path lands.
			OK: true,
		},
		Alerts:     alerts,
		Namespaces: nsOut,
	}, nil
}

// GetNamespaceDashboard returns everything the /ns/:ns page needs in one
// round-trip. Returns (nil, nil) for unknown namespaces; handler maps to 404.
func (s *Service) GetNamespaceDashboard(ctx context.Context, namespace string) (*NamespaceDashboardResponse, error) {
	now := s.nowFn()

	cfg, err := s.repo.GetNamespace(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("get namespace: %w", err)
	}
	if cfg == nil {
		return nil, nil
	}

	runs, _, _, err := s.repo.GetBatchRunLogs(ctx, namespace, "", "", 12, 0)
	if err != nil {
		return nil, fmt.Errorf("get last 12 batch runs: %w", err)
	}
	lastRuns := make([]BatchRunSummary, 0, len(runs))
	for _, r := range runs {
		lastRuns = append(lastRuns, BatchRunSummaryFromLog(r))
	}

	eventCounts, err := s.repo.GetRecentEventCounts(ctx, 24)
	if err != nil {
		return nil, fmt.Errorf("get event counts: %w", err)
	}

	qdrant, err := s.GetQdrant(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("get qdrant: %w", err)
	}

	return &NamespaceDashboardResponse{
		Namespace:       namespace,
		GeneratedAt:     now.UTC(),
		Config:          *cfg,
		LastRuns:        lastRuns,
		Catalog:         CatalogBacklog{}, // Phase 2 wires real backlog
		Events24h:       eventCounts[namespace],
		EventsPerMinNow: s.eventsPerMin(namespace),
		Qdrant:          *qdrant,
		TrendingTTLSec:  0, // Phase 2/3 wires trending TTL probe
	}, nil
}

// eventsPerMin converts the rate tracker's 1-minute events/sec for a namespace
// into events/min. Returns 0 when no tracker is wired (e.g. in tests) or the
// namespace has no recent ingest.
func (s *Service) eventsPerMin(namespace string) float64 {
	if s.eventRate == nil {
		return 0
	}
	return s.eventRate.RatePerSec(namespace, time.Minute) * 60
}

// deriveCronHeartbeat picks the most recent cron run across all namespaces
// and reports the wall-clock lag against the server's idea of "now".
//
// lagSeconds = now - max(lastRun.StartedAt). When no runs exist, ok=false
// and lagSeconds=0; UI renders an "idle" indicator. Threshold for the
// cron_lag alert is owned by buildAlerts, not here.
func deriveCronHeartbeat(lastRuns map[string]BatchRunLog, now time.Time) CronHeartbeat {
	if len(lastRuns) == 0 {
		return CronHeartbeat{OK: false}
	}
	var latest *time.Time
	for _, run := range lastRuns {
		started := run.StartedAt
		if run.TriggerSource != string(batchrun.TriggerCron) {
			continue
		}
		if latest == nil || started.After(*latest) {
			s := started
			latest = &s
		}
	}
	if latest == nil {
		return CronHeartbeat{OK: false}
	}
	lag := int(now.Sub(*latest).Seconds())
	if lag < 0 {
		lag = 0
	}
	return CronHeartbeat{
		LastRunAt:  latest,
		LagSeconds: lag,
		OK:         true,
	}
}

// deriveNamespaceStatus mirrors GetNamespacesOverview's existing logic so the
// new /overview endpoint stays bug-compatible with the legacy ?include=overview
// view. UI keys per-row indicators off this string.
func deriveNamespaceStatus(lastRuns map[string]BatchRunLog, eventCounts map[string]int, ns string) NamespaceStatus {
	run, ok := lastRuns[ns]
	if !ok {
		return NSStatusCold
	}
	if !run.Success {
		return NSStatusDegraded
	}
	if eventCounts[ns] > 0 {
		return NSStatusActive
	}
	return NSStatusIdle
}

// buildAlerts evaluates the Phase 1 alert rules. Thresholds are pinned in
// BUILD_PLAN §3.1. Rules that need data sources not yet wired (catalog
// dead-letter growth, embedder heartbeat, consumer lag) stay quiet here and
// land in their respective phases.
func buildAlerts(namespaces []NamespaceConfig, lastRuns map[string]BatchRunLog, now time.Time) []Alert {
	alerts := make([]Alert, 0)
	for _, ns := range namespaces {
		run, ok := lastRuns[ns.Namespace]
		if !ok {
			continue
		}
		if !run.Success && run.ErrorMessage != nil && *run.ErrorMessage != batchrun.OperatorCancelledMessage {
			alerts = append(alerts, Alert{
				Level:     "warn",
				Namespace: ns.Namespace,
				Kind:      "run_failed",
				Message:   fmt.Sprintf("last batch run failed: %s", *run.ErrorMessage),
			})
		}
		// Stale-cron alert: any namespace whose latest cron run is older
		// than 2× the documented interval (15 min default).
		if run.TriggerSource == string(batchrun.TriggerCron) && now.Sub(run.StartedAt) > 15*time.Minute {
			alerts = append(alerts, Alert{
				Level:     "warn",
				Namespace: ns.Namespace,
				Kind:      "cron_lag",
				Message:   fmt.Sprintf("no cron run in %dm", int(now.Sub(run.StartedAt).Minutes())),
			})
		}
	}
	return alerts
}
