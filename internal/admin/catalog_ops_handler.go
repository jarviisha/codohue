package admin

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jarviisha/codohue/internal/core/httpapi"
)

// TriggerReEmbed handles POST /api/admin/v1/namespaces/{ns}/catalog/re-embed.
//
// Status code mapping:
//
//	202 Accepted              — re-embed orchestration kicked off; Location
//	                            header points at the batch run record.
//	404 Not Found             — namespace does not exist OR catalog_enabled=false
//	                            (same body — see FR-008).
//	409 Conflict              — a re-embed is already in progress.
//	500 Internal Server Error — unexpected DB / Redis error.
//	503 Service Unavailable   — catalog feature not wired in this deployment.
func (h *Handler) TriggerReEmbed(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	if ns == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "missing_namespace", "ns is required")
		return
	}

	resp, err := h.svc.TriggerReEmbed(r.Context(), ns)
	if err != nil {
		switch {
		case errors.Is(err, ErrCatalogStrategyPickerUnavailable):
			httpapi.WriteError(w, http.StatusServiceUnavailable, "catalog_unavailable",
				"catalog auto-embedding is not wired in this deployment")
		case errors.Is(err, ErrReembedAlreadyRunning):
			httpapi.WriteError(w, http.StatusConflict, "reembed_running",
				"a re-embed is already in progress for this namespace")
		default:
			httpapi.WriteError(w, http.StatusInternalServerError, "internal_error",
				"could not trigger re-embed")
		}
		return
	}
	if resp == nil {
		// nil result + nil error = namespace not found OR catalog disabled.
		httpapi.WriteError(w, http.StatusNotFound, "not_found",
			"namespace not found or catalog auto-embedding not enabled")
		return
	}

	w.Header().Set("Location",
		"/api/admin/v1/namespaces/"+ns+"/batch-runs/"+strconv.FormatInt(resp.BatchRunID, 10))
	httpapi.WriteJSON(w, http.StatusAccepted, resp)
}

// ListCatalogItems handles GET /api/admin/v1/namespaces/{ns}/catalog/items.
//
// Query params:
//
//	state      — filter by state ("pending", "in_flight", "embedded",
//	             "failed", "dead_letter", or "all"); default "all".
//	limit      — page size, default 50, capped at 500.
//	offset     — pagination offset, default 0.
//	object_id  — substring filter over object_id (case-insensitive).
func (h *Handler) ListCatalogItems(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	q := r.URL.Query()

	state := q.Get("state")
	if state == "" {
		state = "all"
	}

	limit := 50
	if lStr := q.Get("limit"); lStr != "" {
		l, err := strconv.Atoi(lStr)
		if err != nil || l < 1 || l > 500 {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_param",
				"limit must be between 1 and 500")
			return
		}
		limit = l
	}
	offset := 0
	if oStr := q.Get("offset"); oStr != "" {
		o, err := strconv.Atoi(oStr)
		if err != nil || o < 0 {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_param",
				"offset must be a non-negative integer")
			return
		}
		offset = o
	}
	objectIDFilter := q.Get("object_id")

	resp, err := h.svc.ListCatalogItems(r.Context(), ns, state, limit, offset, objectIDFilter)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error",
			"could not list catalog items")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, resp)
}

// GetCatalogItem handles GET /api/admin/v1/namespaces/{ns}/catalog/items/{id}.
// Returns 200 with the full record, or 404 when the row is not found.
func (h *Handler) GetCatalogItem(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	idStr := chi.URLParam(r, "id")

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_id",
			"id must be a positive integer")
		return
	}

	item, err := h.svc.GetCatalogItem(r.Context(), ns, id)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error",
			"could not get catalog item")
		return
	}
	if item == nil {
		httpapi.WriteError(w, http.StatusNotFound, "not_found",
			"catalog item not found")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, item)
}

// RedriveCatalogItem handles POST /api/admin/v1/namespaces/{ns}/catalog/items/{id}/redrive.
// Returns 202 on success, 404 when the row is not found OR is in a state
// that cannot be redriven (only `failed` and `dead_letter` are eligible).
func (h *Handler) RedriveCatalogItem(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	idStr := chi.URLParam(r, "id")

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_id",
			"id must be a positive integer")
		return
	}

	resp, err := h.svc.RedriveCatalogItem(r.Context(), ns, id)
	if err != nil {
		switch {
		case errors.Is(err, ErrCatalogStrategyPickerUnavailable):
			httpapi.WriteError(w, http.StatusServiceUnavailable, "catalog_unavailable",
				"catalog auto-embedding is not wired in this deployment")
		default:
			httpapi.WriteError(w, http.StatusInternalServerError, "internal_error",
				"could not redrive catalog item")
		}
		return
	}
	if resp == nil {
		httpapi.WriteError(w, http.StatusNotFound, "not_found",
			"catalog item not found or not in a redrivable state")
		return
	}
	httpapi.WriteJSON(w, http.StatusAccepted, resp)
}

// BulkRedriveDeadletter handles POST /api/admin/v1/namespaces/{ns}/catalog/items/redrive-deadletter.
// Returns 200 with a count of redriven items.
func (h *Handler) BulkRedriveDeadletter(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	if ns == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "missing_namespace", "ns is required")
		return
	}

	resp, err := h.svc.BulkRedriveDeadletter(r.Context(), ns)
	if err != nil {
		switch {
		case errors.Is(err, ErrCatalogStrategyPickerUnavailable):
			httpapi.WriteError(w, http.StatusServiceUnavailable, "catalog_unavailable",
				"catalog auto-embedding is not wired in this deployment")
		default:
			httpapi.WriteError(w, http.StatusInternalServerError, "internal_error",
				"could not bulk redrive dead-letter items")
		}
		return
	}
	if resp == nil {
		httpapi.WriteError(w, http.StatusNotFound, "not_found",
			"namespace not found or catalog auto-embedding not enabled")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, resp)
}

// DeleteCatalogItem handles DELETE /api/admin/v1/namespaces/{ns}/catalog/items/{id}.
// Idempotent — deleting a non-existent item still returns 204.
func (h *Handler) DeleteCatalogItem(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	idStr := chi.URLParam(r, "id")

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_id",
			"id must be a positive integer")
		return
	}

	if err := h.svc.DeleteCatalogItem(r.Context(), ns, id); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error",
			"could not delete catalog item")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
