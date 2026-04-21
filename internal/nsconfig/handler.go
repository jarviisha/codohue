package nsconfig

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jarviisha/codohue/internal/core/httpapi"
)

type nsConfigUpserter interface {
	Upsert(ctx context.Context, namespace string, req *UpsertRequest) (*UpsertResponse, error)
}

// Handler handles HTTP requests for namespace configuration.
type Handler struct {
	service nsConfigUpserter
}

// NewHandler creates a new Handler with the given namespace config service.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Upsert handles PUT /v1/config/namespaces/{namespace} — creates or updates the namespace config.
func (h *Handler) Upsert(w http.ResponseWriter, r *http.Request) {
	namespace := chi.URLParam(r, "namespace")

	var req UpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}

	resp, err := h.service.Upsert(r.Context(), namespace, &req)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("[nsconfig] encode response: %v", err)
	}
}
