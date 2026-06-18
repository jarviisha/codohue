package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jarviisha/codohue/internal/core/httpapi"
	"github.com/jarviisha/codohue/internal/infra/metrics"
)

type eventProcessorHTTP interface {
	Process(ctx context.Context, payload *EventPayload) (int64, error)
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
// the request body is silently overwritten (the path is the single source of
// truth).
func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request) {
	namespace := chi.URLParam(r, "ns")
	if namespace == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "missing_namespace", "ns is required")
		return
	}

	var payload EventPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		metrics.IngestErrorsTotal.WithLabelValues(namespace, "decode").Inc()
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}
	payload.Namespace = namespace

	eventID, err := h.service.Process(r.Context(), &payload)
	if err != nil {
		if isClientPayloadError(err) {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_event", err.Error())
			return
		}
		log.Printf("[ingest] process event: %v", err)
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	// 202 body carries the generated id so callers (e.g. the admin "inject
	// test event" action) can highlight the freshly-landed row. Existing
	// data-plane clients ignore the body — adding it is non-breaking.
	httpapi.WriteJSON(w, http.StatusAccepted, map[string]int64{"event_id": eventID})
}

func isClientPayloadError(err error) bool {
	return errors.Is(err, ErrInvalidPayload) || errors.Is(err, ErrUnknownAction)
}
