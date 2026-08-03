package catalog

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jarviisha/codohue/internal/core/httpapi"
	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// catalogIngester abstracts the Service for the handler layer; tests use
// it to inject canned errors without exercising the full service.
type catalogIngester interface {
	Ingest(ctx context.Context, namespace string, req *IngestRequest) (*Item, error)
	IngestBatch(ctx context.Context, namespace string, req *BatchIngestRequest) (*codohuetypes.CatalogBatchIngestResponse, error)
	ListObjects(ctx context.Context, namespace string, changedSince *time.Time, limit, offset int) (*codohuetypes.CatalogObjectsResponse, error)
}

// Handler exposes the data-plane catalog routes under
// /v1/namespaces/{ns}/catalog.
type Handler struct {
	service catalogIngester
}

// NewHandler creates a new Handler with the given Service.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Ingest handles POST /v1/namespaces/{ns}/catalog. The namespace is taken
// exclusively from the URL path; the body has no namespace field, so a stray
// one is rejected as an unknown field rather than silently ignored (strict
// request decoding locks the wire contract).
//
// Status code mapping per contracts/rest-api.md:
//
//	202 Accepted             — happy path
//	400 Bad Request          — invalid JSON / unknown field / missing object_id / bad request shape
//	404 Not Found            — namespace missing OR not enabled (same body to
//	                           avoid leaking namespace existence)
//	413 Payload Too Large    — len(content) exceeds catalog_max_content_bytes
//	422 Unprocessable Entity — content empty after trimming
//	500 Internal Server Error — unexpected server-side failure
func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	if ns == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "missing_namespace", "ns is required")
		return
	}

	var req IngestRequest
	if err := httpapi.DecodeStrict(r.Body, &req); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}

	item, err := h.service.Ingest(r.Context(), ns, &req)
	if err != nil {
		h.writeError(w, r, ns, err)
		return
	}

	_ = item // body is empty for 202 Accepted
	w.WriteHeader(http.StatusAccepted)
}

// BatchIngest handles POST /v1/namespaces/{ns}/catalog/batch: up to
// codohuetypes.CatalogBatchMaxItems items per request, validated
// independently, with per-item results in request order. Namespace-level
// failures map exactly like the single-item endpoint (404 masking both
// missing and not-enabled); everything item-level is a 202 with the outcome
// in the body.
func (h *Handler) BatchIngest(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	if ns == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "missing_namespace", "ns is required")
		return
	}

	var req BatchIngestRequest
	if err := httpapi.DecodeStrict(r.Body, &req); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}

	resp, err := h.service.IngestBatch(r.Context(), ns, &req)
	if err != nil {
		h.writeError(w, r, ns, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusAccepted, resp)
}

// ListObjects handles GET /v1/namespaces/{ns}/catalog/objects — the
// reconciliation read (?changed_since=RFC3339&limit=&offset=). Ordered by
// updated_at ascending so a repair pass pages forward and resumes from the
// last timestamp it saw.
func (h *Handler) ListObjects(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	if ns == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "missing_namespace", "ns is required")
		return
	}

	var changedSince *time.Time
	if raw := r.URL.Query().Get("changed_since"); raw != "" {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "changed_since must be RFC3339")
			return
		}
		changedSince = &t
	}
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "limit must be a positive integer")
			return
		}
		limit = min(n, 1000)
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "offset must be a non-negative integer")
			return
		}
		offset = n
	}

	resp, err := h.service.ListObjects(r.Context(), ns, changedSince, limit, offset)
	if err != nil {
		h.writeError(w, r, ns, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, ns string, err error) {
	switch {
	case errors.Is(err, ErrInvalidRequest):
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
	case errors.Is(err, ErrEmptyContent):
		httpapi.WriteError(w, http.StatusUnprocessableEntity, "empty_content", err.Error())
	case errors.Is(err, ErrContentTooLarge):
		httpapi.WriteError(w, http.StatusRequestEntityTooLarge, "content_too_large", err.Error())
	case errors.Is(err, ErrNamespaceNotFound), errors.Is(err, ErrNamespaceNotEnabled):
		// Same status + body for both so unauthenticated probes can't
		// enumerate namespaces.
		httpapi.WriteError(w, http.StatusNotFound, "namespace_not_enabled",
			"namespace not found or catalog auto-embedding not enabled")
	default:
		slog.ErrorContext(r.Context(), "catalog ingest failed",
			slog.String("namespace", ns),
			slog.String("error", err.Error()),
		)
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
