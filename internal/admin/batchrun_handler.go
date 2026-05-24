package admin

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jarviisha/codohue/internal/core/httpapi"
)

// parseRunID is the shared id-parser for /batch-runs/{id} routes. Returns the
// id or writes a 400 + false and lets the caller bail out.
func parseRunID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "id must be a positive integer")
		return 0, false
	}
	return id, true
}

// GetBatchRunDetail handles GET /api/admin/v1/batch-runs/{id}.
// Returns 200 with BatchRunDetail or 404 if the run does not exist.
func (h *Handler) GetBatchRunDetail(w http.ResponseWriter, r *http.Request) {
	id, ok := parseRunID(w, r)
	if !ok {
		return
	}
	detail, err := h.svc.GetBatchRunDetail(r.Context(), id)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "could not load batch run")
		return
	}
	if detail == nil {
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "batch run not found")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, detail)
}

// CancelBatchRun handles POST /api/admin/v1/batch-runs/{id}/cancel.
// Status mapping:
//
//	200 — cancel requested; body is the refreshed BatchRunSummary
//	404 — run id does not exist
//	409 — run already terminal
func (h *Handler) CancelBatchRun(w http.ResponseWriter, r *http.Request) {
	id, ok := parseRunID(w, r)
	if !ok {
		return
	}
	summary, status, err := h.svc.CancelBatchRun(r.Context(), id)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	switch status {
	case http.StatusNotFound:
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "batch run not found")
	case http.StatusConflict:
		httpapi.WriteError(w, http.StatusConflict, "conflict", "batch run is already terminal")
	default:
		httpapi.WriteJSON(w, http.StatusOK, summary)
	}
}

// RetryBatchRun handles POST /api/admin/v1/batch-runs/{id}/retry.
// Reject rules from BUILD_PLAN §D4:
//
//	404 — original run not found
//	422 — re-embed retries (use catalog re-embed) or deleted namespace
//	409 — original run still in-flight
//	202 — accepted; Location header points to the new run
func (h *Handler) RetryBatchRun(w http.ResponseWriter, r *http.Request) {
	id, ok := parseRunID(w, r)
	if !ok {
		return
	}
	created, status, err := h.svc.RetryBatchRun(r.Context(), id)
	switch status {
	case http.StatusNotFound:
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "batch run not found")
		return
	case http.StatusUnprocessableEntity:
		msg := "retry not allowed"
		if err != nil {
			msg = err.Error()
		}
		httpapi.WriteError(w, http.StatusUnprocessableEntity, "unprocessable", msg)
		return
	case http.StatusConflict:
		httpapi.WriteError(w, http.StatusConflict, "conflict", "original batch run is still in flight")
		return
	}
	if err != nil {
		if errors.Is(err, errBatchRunning) {
			httpapi.WriteError(w, http.StatusConflict, "conflict", err.Error())
			return
		}
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if created != nil && created.ID > 0 {
		w.Header().Set("Location", "/api/admin/v1/batch-runs/"+strconv.FormatInt(created.ID, 10))
	}
	httpapi.WriteJSON(w, http.StatusAccepted, created)
}

// GetBatchRunStats handles GET /api/admin/v1/batch-runs/stats?window=&bucket=.
// Window and bucket use Go duration strings ("24h", "1h", "10m"). Defaults
// pin to a 24h window with 1h buckets so the Fleet time-series chart works
// out of the box.
func (h *Handler) GetBatchRunStats(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	window, err := parseDurationDefault(q.Get("window"), 24*time.Hour)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "window: "+err.Error())
		return
	}
	bucket, err := parseDurationDefault(q.Get("bucket"), time.Hour)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "bucket: "+err.Error())
		return
	}
	buckets, err := h.svc.GetBatchRunStats(r.Context(), window, bucket)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if buckets == nil {
		buckets = []BatchRunStatsBucket{}
	}
	httpapi.WriteJSON(w, http.StatusOK, struct {
		WindowSeconds int                   `json:"window_seconds"`
		BucketSeconds int                   `json:"bucket_seconds"`
		Series        []BatchRunStatsBucket `json:"series"`
	}{
		WindowSeconds: int(window.Seconds()),
		BucketSeconds: int(bucket.Seconds()),
		Series:        buckets,
	})
}

func parseDurationDefault(raw string, def time.Duration) (time.Duration, error) {
	if raw == "" {
		return def, nil
	}
	return time.ParseDuration(raw)
}
