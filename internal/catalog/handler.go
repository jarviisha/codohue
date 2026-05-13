package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/jarviisha/codohue/internal/core/httpapi"
)

// catalogIngester abstracts Service.Ingest for the handler layer; tests use
// it to inject canned errors without exercising the full service.
type catalogIngester interface {
	Ingest(ctx context.Context, namespace string, req *IngestRequest) (*Item, error)
}

// Handler exposes POST /v1/namespaces/{ns}/catalog.
type Handler struct {
	service catalogIngester
}

// NewHandler creates a new Handler with the given Service.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Ingest handles POST /v1/namespaces/{ns}/catalog. The namespace is taken
// exclusively from the URL path; any namespace value in the body is ignored
// (consistent with the 003 RESTful redesign).
//
// Status code mapping per contracts/rest-api.md:
//
//	202 Accepted             — happy path
//	400 Bad Request          — invalid JSON / missing object_id / bad request shape
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
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
