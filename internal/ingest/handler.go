package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jarviisha/codohue/internal/core/httpapi"
)

type eventProcessorHTTP interface {
	Process(ctx context.Context, payload *EventPayload) error
}

// Handler handles HTTP event ingestion requests.
type Handler struct {
	service eventProcessorHTTP
}

// NewHandler creates a new ingest HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Ingest handles POST /v1/namespaces/{ns}/events and persists a single event.
// The namespace is taken exclusively from the URL path; any namespace value in
// the request body is silently ignored (the path is the single source of truth).
func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request) {
	namespace := chi.URLParam(r, "ns")
	if namespace == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "missing_namespace", "ns is required")
		return
	}

	var req HTTPIngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}

	payload := EventPayload{
		Namespace:       namespace,
		SubjectID:       req.SubjectID,
		ObjectID:        req.ObjectID,
		Action:          req.Action,
		Timestamp:       req.OccurredAt,
		ObjectCreatedAt: req.ObjectCreatedAt,
		Metadata:        req.Metadata,
	}

	if err := h.service.Process(r.Context(), &payload); err != nil {
		if isClientPayloadError(err) {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_event", err.Error())
			return
		}
		log.Printf("[ingest] process event: %v", err)
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func isClientPayloadError(err error) bool {
	return errors.Is(err, ErrInvalidPayload) || errors.Is(err, ErrUnknownAction)
}
