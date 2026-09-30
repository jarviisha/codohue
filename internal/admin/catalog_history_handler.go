package admin

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jarviisha/codohue/internal/core/httpapi"
)

// parseWindow parses a lookback window and rejects anything under 1s: the
// repository works in whole seconds and would turn a zero window into a 500.
func parseWindow(raw string, def time.Duration) (time.Duration, error) {
	window, err := parseDurationDefault(raw, def)
	if err == nil && window < time.Second {
		err = errors.New("must be at least 1s")
	}
	return window, err
}

// GetCatalogBacklogHistory handles
// GET /api/admin/v1/namespaces/{ns}/catalog/backlog-history?window=1h&bucket=5m
//
// Window is a Go duration string (e.g. "1h", "24h", "168h"). Default 1h —
// matches the Catalog status page's initial chart window. Bucket is optional:
// absent or zero returns raw samples; otherwise samples are downsampled to
// one point per bucket, taking the MAX of each series so backlog peaks survive.
func (h *Handler) GetCatalogBacklogHistory(w http.ResponseWriter, r *http.Request) {
	ns := httpapi.URLParam(r, "ns")
	if ns == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "namespace is required")
		return
	}
	window, err := parseWindow(r.URL.Query().Get("window"), time.Hour)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "window: "+err.Error())
		return
	}
	// Storage math is in whole seconds; a sub-second bucket would truncate
	// to 0 and silently mean "raw samples".
	bucket, err := parseDurationDefault(r.URL.Query().Get("bucket"), 0)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "bucket: "+err.Error())
		return
	}
	if bucket != 0 && bucket < time.Second {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "bucket must be 0 or at least 1s")
		return
	}
	resp, err := h.svc.GetCatalogBacklogHistory(r.Context(), ns, window, bucket)
	if err != nil {
		writeInternalError(w, r, "could not load backlog history", err, slog.String("namespace", ns))
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, resp)
}

// GetCatalogFailuresSummary handles
// GET /api/admin/v1/namespaces/{ns}/catalog/failures-summary?window=24h&limit=10
//
// Window default 24h, limit default 10. Returns top-N failure reasons +
// counts + a sample object_id so operators can drill into a representative
// failed item.
func (h *Handler) GetCatalogFailuresSummary(w http.ResponseWriter, r *http.Request) {
	ns := httpapi.URLParam(r, "ns")
	if ns == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "namespace is required")
		return
	}
	window, err := parseWindow(r.URL.Query().Get("window"), 24*time.Hour)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "window: "+err.Error())
		return
	}
	limit := 10
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 || n > 100 {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "limit must be 1..100")
			return
		}
		limit = n
	}
	resp, err := h.svc.GetCatalogFailuresSummary(r.Context(), ns, window, limit)
	if err != nil {
		writeInternalError(w, r, "could not load failures summary", err, slog.String("namespace", ns))
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, resp)
}
