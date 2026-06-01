package admin

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// GetEventsSummary aggregates ingest for ns over window, bucketed by bucket.
// Per-action and overall rates are events-per-second across the window.
func (s *Service) GetEventsSummary(ctx context.Context, ns string, window, bucket time.Duration) (*EventsSummaryResponse, error) {
	windowSecs := int(window.Seconds())
	if windowSecs < 1 {
		windowSecs = 1
	}
	bucketSecs := int(bucket.Seconds())
	if bucketSecs < 1 {
		bucketSecs = 1
	}

	total, byAction, series, err := s.repo.GetEventsSummary(ctx, ns, windowSecs, bucketSecs)
	if err != nil {
		return nil, fmt.Errorf("aggregate events: %w", err)
	}

	actions := make([]EventsSummaryAction, 0, len(byAction))
	for action, count := range byAction {
		actions = append(actions, EventsSummaryAction{
			Action: action,
			Count:  count,
			Rate:   float64(count) / float64(windowSecs),
		})
	}
	// Highest-volume action first; tie-break by name for a stable order.
	sort.Slice(actions, func(i, j int) bool {
		if actions[i].Count != actions[j].Count {
			return actions[i].Count > actions[j].Count
		}
		return actions[i].Action < actions[j].Action
	})

	return &EventsSummaryResponse{
		WindowSeconds: windowSecs,
		BucketSeconds: bucketSecs,
		Total:         total,
		RatePerSecond: float64(total) / float64(windowSecs),
		ByAction:      actions,
		Series:        series,
	}, nil
}

// GetMetricsSummary returns the curated rolling-window metrics. Ingest rates
// come from the admin-plane event-tail tracker; cron lag is derived from the
// latest cron run. Recommend/embedder live in other processes and are scraped
// by Prometheus directly, so they are intentionally absent here.
func (s *Service) GetMetricsSummary(ctx context.Context) (*MetricsSummaryResponse, error) {
	now := s.nowFn()

	ingest := MetricsSummaryIngest{
		EventsPerSec1m: map[string]float64{},
		EventsPerSec5m: map[string]float64{},
	}
	if s.eventRate != nil {
		ingest.EventsPerSec1m = s.eventRate.RatesPerSec(time.Minute)
		ingest.EventsPerSec5m = s.eventRate.RatesPerSec(5 * time.Minute)
	}

	var lag float64
	if lastRuns, err := s.repo.GetLastBatchRunPerNamespace(ctx); err == nil {
		lag = float64(deriveCronHeartbeat(lastRuns, now).LagSeconds)
	}

	return &MetricsSummaryResponse{
		GeneratedAt: now.UTC().Format(time.RFC3339),
		Ingest:      ingest,
		Cron:        MetricsSummaryCron{BatchLagSeconds: lag},
	}, nil
}
