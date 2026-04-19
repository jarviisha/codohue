package nsconfig

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
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
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	resp, err := h.service.Upsert(r.Context(), namespace, &req)
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("[nsconfig] encode response: %v", err)
	}
}
