package admin

import (
	"context"
	"fmt"
	"time"
)

// GetCatalogBacklogHistory returns the persisted backlog samples for a
// namespace over the requested window. Default window is 1h. A zero bucket
// returns raw samples; a positive bucket downsamples to one point per bucket.
func (s *Service) GetCatalogBacklogHistory(ctx context.Context, namespace string, window, bucket time.Duration) (*CatalogBacklogHistoryResponse, error) {
	windowSec := int(window.Seconds())
	if windowSec <= 0 {
		return nil, fmt.Errorf("invalid window: %v", window)
	}
	bucketSec := int(bucket.Seconds())
	samples, err := s.repo.GetCatalogBacklogHistory(ctx, namespace, windowSec, bucketSec)
	if err != nil {
		return nil, fmt.Errorf("get backlog history: %w", err)
	}
	if samples == nil {
		samples = []CatalogBacklogSample{}
	}
	return &CatalogBacklogHistoryResponse{
		Namespace:     namespace,
		WindowSeconds: windowSec,
		BucketSeconds: bucketSec,
		Samples:       samples,
	}, nil
}

// GetCatalogFailuresSummary returns the top failure reasons bucketed by
// last_error within the window. Default window is 24h, default limit 10.
func (s *Service) GetCatalogFailuresSummary(ctx context.Context, namespace string, window time.Duration, limit int) (*CatalogFailuresSummaryResponse, error) {
	windowSec := int(window.Seconds())
	if windowSec <= 0 {
		return nil, fmt.Errorf("invalid window: %v", window)
	}
	if limit <= 0 {
		limit = 10
	}
	reasons, err := s.repo.GetCatalogFailuresSummary(ctx, namespace, windowSec, limit)
	if err != nil {
		return nil, fmt.Errorf("get failures summary: %w", err)
	}
	if reasons == nil {
		reasons = []CatalogFailureReason{}
	}
	return &CatalogFailuresSummaryResponse{
		Namespace:     namespace,
		WindowSeconds: windowSec,
		Reasons:       reasons,
	}, nil
}
