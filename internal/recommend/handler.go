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
// POST /v1/rank. Requests exceeding this limit receive a 400 Bad Request.
// This prevents unbounded Qdrant filter sizes and per-candidate ID lookups.
const maxCandidates = 500

// KeyValidatorFn validates a Bearer token against a namespace's API key.
// Returns true when the request is authorized.
type KeyValidatorFn func(ctx context.Context, token, namespace string) bool

type recommendSvc interface {
	Recommend(ctx context.Context, req *Request) (*Response, error)
	GetTrending(ctx context.Context, ns string, limit, offset, windowHours int) (*TrendingResponse, error)
	Rank(ctx context.Context, req *RankRequest) (*RankResponse, error)
	StoreObjectEmbedding(ctx context.Context, namespace, objectID string, vector []float32) error
	StoreSubjectEmbedding(ctx context.Context, namespace, subjectID string, vector []float32) error
	DeleteObject(ctx context.Context, namespace, objectID string) error
}

// Handler handles HTTP requests for recommendations.
type Handler struct {
	service     recommendSvc
	validateKey KeyValidatorFn
}

// NewHandler creates a new Handler with the given recommendation service.
// validateKey is used to authorize POST /v1/rank requests by namespace.
func NewHandler(service *Service, validateKey KeyValidatorFn) *Handler {
	return &Handler{service: service, validateKey: validateKey}
}

// Get handles GET /v1/recommendations — returns recommended items for a subject.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	subjectID := q.Get("subject_id")
	namespace := q.Get("namespace")
	h.get(w, r, namespace, subjectID)
}

// GetByNamespace handles GET /v1/namespaces/{ns}/recommendations.
func (h *Handler) GetByNamespace(w http.ResponseWriter, r *http.Request) {
	h.get(w, r, chi.URLParam(r, "ns"), r.URL.Query().Get("subject_id"))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request, namespace, subjectID string) {
	if subjectID == "" || namespace == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "missing_required_fields", "subject_id and namespace are required")
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
		log.Printf("[recommend] encode response: %v", err)
	}
}

// Rank handles POST /v1/rank — scores and ranks a list of candidate items for a subject.
// Auth is validated here (after body parse) because the namespace lives in the request body.
func (h *Handler) Rank(w http.ResponseWriter, r *http.Request) {
	h.rank(w, r, "")
}

// RankByNamespace handles POST /v1/namespaces/{ns}/rank.
func (h *Handler) RankByNamespace(w http.ResponseWriter, r *http.Request) {
	h.rank(w, r, chi.URLParam(r, "ns"))
}

func (h *Handler) rank(w http.ResponseWriter, r *http.Request, pathNamespace string) {
	var req RankRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid request body")
		return
	}

	if pathNamespace != "" {
		if req.Namespace == "" {
			req.Namespace = pathNamespace
		} else if req.Namespace != pathNamespace {
			httpapi.WriteError(w, http.StatusBadRequest, "namespace_mismatch", "namespace in path and body must match")
			return
		}
	}

	if req.SubjectID == "" || req.Namespace == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "missing_required_fields", "subject_id and namespace are required")
		return
	}

	if len(req.Candidates) > maxCandidates {
		httpapi.WriteError(w, http.StatusBadRequest, "too_many_candidates", "candidates exceeds limit of "+strconv.Itoa(maxCandidates))
		return
	}

	if h.validateKey != nil {
		token := extractBearerToken(r)
		if !h.validateKey(r.Context(), token, req.Namespace) {
			httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid or missing bearer token")
			return
		}
	}

	resp, err := h.service.Rank(r.Context(), &req)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "internal server error")
		return
	}

	if err := writeJSON(w, resp); err != nil {
		log.Printf("[recommend] rank: encode response: %v", err)
	}
}

// GetTrending handles GET /v1/trending/{ns} — returns trending items from the Redis ZSET.
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

// StoreObjectEmbedding handles POST /v1/objects/{ns}/{id}/embedding — BYOE for items.
func (h *Handler) StoreObjectEmbedding(w http.ResponseWriter, r *http.Request) {
	h.storeEmbedding(w, r, "object")
}

// StoreSubjectEmbedding handles POST /v1/subjects/{ns}/{id}/embedding — BYOE for users.
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

	if h.validateKey != nil {
		if !h.validateKey(r.Context(), extractBearerToken(r), ns) {
			httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid or missing bearer token")
			return
		}
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
		// Distinguish dimension mismatch (400) from infra errors (500).
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

// DeleteObject handles DELETE /v1/objects/{ns}/{id} — removes an object from Qdrant.
// Returns 204 on success. The operation is idempotent: deleting a non-existent object
// also returns 204.
func (h *Handler) DeleteObject(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	id := chi.URLParam(r, "id")

	if ns == "" || id == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "missing_required_fields", "ns and id are required")
		return
	}

	if h.validateKey != nil {
		if !h.validateKey(r.Context(), extractBearerToken(r), ns) {
			httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid or missing bearer token")
			return
		}
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

func extractBearerToken(r *http.Request) string {
	v := r.Header.Get("Authorization")
	if len(v) > 7 && v[:7] == "Bearer " {
		return v[7:]
	}
	return ""
}
