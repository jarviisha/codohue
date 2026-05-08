package recommend

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jarviisha/codohue/internal/core/httpapi"
)

// maxCandidates is the hard upper bound on the number of candidates accepted by
// POST /v1/namespaces/{ns}/rankings. Requests exceeding this limit receive a
// 400 Bad Request. This prevents unbounded Qdrant filter sizes and per-candidate
// ID lookups.
const maxCandidates = 500

type recommendSvc interface {
	Recommend(ctx context.Context, req *Request) (*Response, error)
	GetTrending(ctx context.Context, ns string, limit, offset, windowHours int) (*TrendingResponse, error)
	Rank(ctx context.Context, req *RankRequest, namespace string) (*RankResponse, error)
	StoreObjectEmbedding(ctx context.Context, namespace, objectID string, vector []float32) error
	StoreSubjectEmbedding(ctx context.Context, namespace, subjectID string, vector []float32) error
	DeleteObject(ctx context.Context, namespace, objectID string) error
}

// Handler handles HTTP requests for recommendations.
//
// Authentication is enforced by middleware on the parent route group;
// handlers in this package do not perform in-handler auth checks.
type Handler struct {
	service recommendSvc
}

// NewHandler creates a new Handler with the given recommendation service.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// GetSubjectRecommendations handles GET /v1/namespaces/{ns}/subjects/{id}/recommendations.
// It returns collaborative-filtering recommendations for a subject as a typed
// response { items, total, source, generated_at }.
func (h *Handler) GetSubjectRecommendations(w http.ResponseWriter, r *http.Request) {
	namespace := chi.URLParam(r, "ns")
	subjectID := chi.URLParam(r, "id")

	if namespace == "" || subjectID == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "missing_required_fields", "namespace and subject id are required")
		return
	}

	q := r.URL.Query()

	limit := 20
	if l := q.Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n <= 0 {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_limit", "invalid limit")
			return
		}
		limit = n
	}

	offset := 0
	if o := q.Get("offset"); o != "" {
		n, err := strconv.Atoi(o)
		if err != nil || n < 0 {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_offset", "invalid offset")
			return
		}
		offset = n
	}

	resp, err := h.service.Recommend(r.Context(), &Request{
		SubjectID: subjectID,
		Namespace: namespace,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	if err := writeJSON(w, resp); err != nil {
		log.Printf("[recommend] get: encode response: %v", err)
	}
}

// Rank handles POST /v1/namespaces/{ns}/rankings — scores and ranks a list of
// candidate items for a subject.
func (h *Handler) Rank(w http.ResponseWriter, r *http.Request) {
	namespace := chi.URLParam(r, "ns")
	if namespace == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "missing_namespace", "namespace is required")
		return
	}

	var req RankRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}

	if req.SubjectID == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "missing_required_fields", "subject_id is required")
		return
	}

	if len(req.Candidates) > maxCandidates {
		httpapi.WriteError(w, http.StatusBadRequest, "too_many_candidates", "candidates exceeds limit of "+strconv.Itoa(maxCandidates))
		return
	}

	resp, err := h.service.Rank(r.Context(), &req, namespace)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	if err := writeJSON(w, resp); err != nil {
		log.Printf("[recommend] rank: encode response: %v", err)
	}
}

// GetTrending handles GET /v1/namespaces/{ns}/trending — returns trending items
// from the Redis ZSET.
func (h *Handler) GetTrending(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	if ns == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "missing_namespace", "ns is required")
		return
	}

	q := r.URL.Query()

	limit := 50
	if l := q.Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n <= 0 {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_limit", "invalid limit")
			return
		}
		limit = n
	}

	offset := 0
	if o := q.Get("offset"); o != "" {
		n, err := strconv.Atoi(o)
		if err != nil || n < 0 {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_offset", "invalid offset")
			return
		}
		offset = n
	}

	windowHours := 0
	if wh := q.Get("window_hours"); wh != "" {
		n, err := strconv.Atoi(wh)
		if err != nil || n <= 0 {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_window_hours", "invalid window_hours")
			return
		}
		windowHours = n
	}

	resp, err := h.service.GetTrending(r.Context(), ns, limit, offset, windowHours)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	if err := writeJSON(w, resp); err != nil {
		log.Printf("[recommend] trending: encode response: %v", err)
	}
}

// StoreObjectEmbedding handles PUT /v1/namespaces/{ns}/objects/{id}/embedding —
// idempotent BYOE storage for items.
func (h *Handler) StoreObjectEmbedding(w http.ResponseWriter, r *http.Request) {
	h.storeEmbedding(w, r, "object")
}

// StoreSubjectEmbedding handles PUT /v1/namespaces/{ns}/subjects/{id}/embedding —
// idempotent BYOE storage for users.
func (h *Handler) StoreSubjectEmbedding(w http.ResponseWriter, r *http.Request) {
	h.storeEmbedding(w, r, "subject")
}

func (h *Handler) storeEmbedding(w http.ResponseWriter, r *http.Request, entityType string) {
	ns := chi.URLParam(r, "ns")
	id := chi.URLParam(r, "id")

	if ns == "" || id == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "missing_required_fields", "ns and id are required")
		return
	}

	var req EmbeddingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.Vector) == 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid request body: vector required")
		return
	}

	var storeErr error
	if entityType == "object" {
		storeErr = h.service.StoreObjectEmbedding(r.Context(), ns, id, req.Vector)
	} else {
		storeErr = h.service.StoreSubjectEmbedding(r.Context(), ns, id, req.Vector)
	}

	if storeErr != nil {
		if isDimMismatch(storeErr) {
			httpapi.WriteError(w, http.StatusBadRequest, "embedding_dimension_mismatch", storeErr.Error())
			return
		}
		log.Printf("[recommend] store %s embedding: %v", entityType, storeErr)
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// DeleteObject handles DELETE /v1/namespaces/{ns}/objects/{id} — removes an
// object from Qdrant. Idempotent: deleting a non-existent object also returns 204.
func (h *Handler) DeleteObject(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	id := chi.URLParam(r, "id")

	if ns == "" || id == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "missing_required_fields", "ns and id are required")
		return
	}

	if err := h.service.DeleteObject(r.Context(), ns, id); err != nil {
		log.Printf("[recommend] delete object: %v", err)
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func isDimMismatch(err error) bool {
	if err == nil {
		return false
	}
	return strings.HasPrefix(err.Error(), "embedding dimension mismatch")
}

func writeJSON(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		return fmt.Errorf("encode json response: %w", err)
	}
	return nil
}
