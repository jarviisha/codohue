package admin

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jarviisha/codohue/internal/core/httpapi"
)

// GetOverview handles GET /api/admin/v1/overview — Fleet aggregate.
func (h *Handler) GetOverview(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.GetOverview(r.Context())
	if err != nil {
		writeInternalError(w, r, "could not build overview", err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, out)
}

// GetNamespaceDashboard handles GET /api/admin/v1/namespaces/{ns}/dashboard.
// 404 when the namespace does not exist.
func (h *Handler) GetNamespaceDashboard(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	if ns == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "namespace is required")
		return
	}
	out, err := h.svc.GetNamespaceDashboard(r.Context(), ns)
	if err != nil {
		writeInternalError(w, r, "could not build dashboard", err, slog.String("namespace", ns))
		return
	}
	if out == nil {
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "namespace not found")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, out)
}
