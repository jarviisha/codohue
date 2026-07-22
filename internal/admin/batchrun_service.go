package admin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jarviisha/codohue/internal/core/batchrun"
)

// errRetryReembedUnsupported guards the retry endpoint from spawning re-embed
// runs; that path needs catalog config validation that the CF retry shortcut
// does not perform. Operators trigger a fresh re-embed from the catalog page.
var errRetryReembedUnsupported = errors.New("re-embed runs cannot be retried via batch-runs API — use catalog re-embed instead")

// GetBatchRunDetail fetches one run with full phases + log_lines + target_strategy.
// Returns (nil, nil) when the row does not exist; the handler maps that to 404.
func (s *Service) GetBatchRunDetail(ctx context.Context, id int64) (*BatchRunDetail, error) {
	row, err := s.repo.GetBatchRunByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get batch run: %w", err)
	}
	if row == nil {
		return nil, nil
	}
	d := BatchRunDetailFromLog(*row)
	return &d, nil
}

// CancelBatchRun flags an in-flight run for cancel. Cron picks up the flag at
// the next phase boundary and finalises the row.
//
// Returns the resulting status code mapped one-to-one to the HTTP handler:
//
//	200 — cancel requested, row was in-flight
//	404 — run id does not exist
//	409 — run already terminal (completed_at IS NOT NULL)
func (s *Service) CancelBatchRun(ctx context.Context, id int64) (*BatchRunSummary, int, error) {
	result, err := s.repo.RequestCancel(ctx, id)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("request cancel: %w", err)
	}
	switch result {
	case RequestCancelNotFound:
		return nil, http.StatusNotFound, nil
	case RequestCancelAlreadyTerminal:
		return nil, http.StatusConflict, nil
	}

	row, err := s.repo.GetBatchRunByID(ctx, id)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("re-fetch after cancel: %w", err)
	}
	if row == nil {
		// Theoretically unreachable — RequestCancel just confirmed existence.
		return nil, http.StatusNotFound, nil
	}
	summary := BatchRunSummaryFromLog(*row)
	return &summary, http.StatusOK, nil
}

// RetryBatchRun queues a fresh CF run on the same namespace. Reject rules
// from BUILD_PLAN §D4:
//
//	404 — original run not found
//	422 — re-embed retries are not supported here
//	422 — original namespace has been deleted
//	409 — original run is still in-flight (caller should wait)
//
// On success the handler returns 202 + Location header.
func (s *Service) RetryBatchRun(ctx context.Context, id int64) (*BatchRunCreateResponse, int, error) {
	row, err := s.repo.GetBatchRunByID(ctx, id)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("get batch run: %w", err)
	}
	if row == nil {
		return nil, http.StatusNotFound, nil
	}
	if row.TriggerSource == string(batchrun.TriggerReembed) {
		return nil, http.StatusUnprocessableEntity, errRetryReembedUnsupported
	}
	if row.CompletedAt == nil {
		return nil, http.StatusConflict, fmt.Errorf("%w for namespace %s", errBatchRunning, row.Namespace)
	}
	nsConfig, err := s.repo.GetNamespace(ctx, row.Namespace)
	if err != nil {
		return nil, http.StatusInternalServerError, fmt.Errorf("get namespace: %w", err)
	}
	if nsConfig == nil {
		return nil, http.StatusUnprocessableEntity, fmt.Errorf("namespace %s no longer exists", row.Namespace)
	}

	// CreateBatchRun is the existing manual-CF path. It guards against a
	// concurrent run on the same namespace via the cross-process compute
	// lock, so we don't duplicate that protection here.
	created, err := s.CreateBatchRun(ctx, row.Namespace)
	if err != nil {
		if errors.Is(err, errBatchRunning) {
			return nil, http.StatusConflict, err
		}
		return nil, http.StatusInternalServerError, err
	}
	if created == nil {
		// CreateBatchRun returns nil when the namespace doesn't exist; we
		// already checked above, so this would be a race we cannot service.
		return nil, http.StatusUnprocessableEntity, fmt.Errorf("namespace %s no longer exists", row.Namespace)
	}
	return created, http.StatusAccepted, nil
}

// GetBatchRunStats returns time-series buckets of terminal runs across the
// given window. Invalid args (non-positive durations) bubble out as 400.
func (s *Service) GetBatchRunStats(ctx context.Context, window, bucket time.Duration) ([]BatchRunStatsBucket, error) {
	windowSec := int(window.Seconds())
	bucketSec := int(bucket.Seconds())
	if windowSec <= 0 || bucketSec <= 0 || bucketSec > windowSec {
		return nil, fmt.Errorf("invalid window/bucket: window=%v bucket=%v", window, bucket)
	}
	return s.repo.GetBatchRunStats(ctx, windowSec, bucketSec)
}
