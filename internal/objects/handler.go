package objects

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jarviisha/codohue/internal/core/httpapi"
	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// objectsUpserter abstracts Service.Upsert so handler tests can inject
// canned errors without standing up the service.
type objectsUpserter interface {
	Upsert(ctx context.Context, namespace, objectID string, req *UpsertRequest) (*Object, error)
}

// Handler exposes PUT /v1/namespaces/{ns}/objects/{id}.
type Handler struct {
	service objectsUpserter
}

// NewHandler creates a new Handler.
func NewHandler(service objectsUpserter) *Handler {
	return &Handler{service: service}
}

// Upsert handles PUT /v1/namespaces/{ns}/objects/{id}.
//
// Idempotent by construction: the same body twice leaves the same row. The
// namespace and object id come exclusively from the URL path, so a body
// repeating either is rejected as an unknown field rather than silently
// ignored.
//
//	200 OK          — stored
//	400 Bad Request — invalid JSON, unknown field, or missing path params
func (h *Handler) Upsert(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	id := chi.URLParam(r, "id")

	var req UpsertRequest
	if err := httpapi.DecodeStrict(r.Body, &req); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}

	obj, err := h.service.Upsert(r.Context(), ns, id, &req)
	if err != nil {
		if errors.Is(err, ErrInvalidRequest) {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "could not store object metadata")
		return
	}

	httpapi.WriteJSON(w, http.StatusOK, codohuetypes.ObjectResponse{
		Namespace:       obj.Namespace,
		ObjectID:        obj.ObjectID,
		AuthorSubjectID: obj.AuthorSubjectID,
		UpdatedAt:       obj.UpdatedAt.UTC().Format(time.RFC3339),
	})
}
