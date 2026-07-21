package admin

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jarviisha/codohue/internal/core/httpapi"
)

// GetCatalogBacklogHistory handles
// GET /api/admin/v1/namespaces/{ns}/catalog/backlog-history?window=1h
//
// Window is a Go duration string (e.g. "1h", "24h", "7d"). Default 1h —
// matches the Catalog status page's initial chart window.
func (h *Handler) GetCatalogBacklogHistory(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	if ns == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "namespace is required")
		return
	}
	window, err := parseDurationDefault(r.URL.Query().Get("window"), time.Hour)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "window: "+err.Error())
		return
	}
	resp, err := h.svc.GetCatalogBacklogHistory(r.Context(), ns, window)
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
	ns := chi.URLParam(r, "ns")
	if ns == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "namespace is required")
		return
	}
	window, err := parseDurationDefault(r.URL.Query().Get("window"), 24*time.Hour)
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
