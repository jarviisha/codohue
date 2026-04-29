package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jarviisha/codohue/internal/core/httpapi"
)

// adminSvc is the service interface used by Handler.
type adminSvc interface {
	GetHealth(ctx context.Context) (*HealthResponse, int, error)
	ListNamespaces(ctx context.Context) ([]NamespaceConfig, error)
	GetNamespace(ctx context.Context, namespace string) (*NamespaceConfig, error)
	GetNamespacesOverview(ctx context.Context) (*NamespacesOverviewResponse, error)
	UpsertNamespace(ctx context.Context, namespace string, body io.Reader) (*NamespaceUpsertResponse, int, error)
	GetBatchRuns(ctx context.Context, namespace string, limit int) ([]BatchRunLog, error)
	DebugRecommend(ctx context.Context, req *RecommendDebugRequest) (*RecommendDebugResponse, int, error)
	GetTrending(ctx context.Context, namespace string, limit, offset, windowHours int) (*TrendingAdminResponse, error)
	GetSubjectProfile(ctx context.Context, namespace, subjectID string) (*SubjectProfileResponse, error)
	GetQdrantStats(ctx context.Context, namespace string) (*QdrantStatsResponse, error)
}

// Handler handles HTTP requests for the admin API.
type Handler struct {
	svc    adminSvc
	apiKey string
}

// NewHandler creates a new Handler.
func NewHandler(svc adminSvc, apiKey string) *Handler {
	return &Handler{svc: svc, apiKey: apiKey}
}

// Login handles POST /api/auth/login.
func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if req.APIKey != h.apiKey {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid api key")
		return
	}

	token, err := createSessionToken(h.apiKey)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "could not create session")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   int((8 * time.Hour).Seconds()),
		SameSite: http.SameSiteLaxMode,
	})
	httpapi.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Logout handles DELETE /api/auth/logout.
func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	httpapi.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// GetHealth handles GET /api/admin/v1/health.
func (h *Handler) GetHealth(w http.ResponseWriter, r *http.Request) {
	health, statusCode, err := h.svc.GetHealth(r.Context())
	if err != nil {
		httpapi.WriteError(w, http.StatusBadGateway, "proxy_error", "could not reach api")
		return
	}
	httpapi.WriteJSON(w, statusCode, health)
}

// ListNamespaces handles GET /api/admin/v1/namespaces.
func (h *Handler) ListNamespaces(w http.ResponseWriter, r *http.Request) {
	namespaces, err := h.svc.ListNamespaces(r.Context())
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "could not list namespaces")
		return
	}
	if namespaces == nil {
		namespaces = []NamespaceConfig{}
	}
	httpapi.WriteJSON(w, http.StatusOK, NamespacesListResponse{Namespaces: namespaces})
}

// GetNamespace handles GET /api/admin/v1/namespaces/{ns}.
func (h *Handler) GetNamespace(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	cfg, err := h.svc.GetNamespace(r.Context(), ns)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "could not get namespace")
		return
	}
	if cfg == nil {
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "namespace not found")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, cfg)
}

// UpsertNamespace handles PUT /api/admin/v1/namespaces/{ns}.
func (h *Handler) UpsertNamespace(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")

	body, err := io.ReadAll(r.Body)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "could not read body")
		return
	}

	var req NamespaceUpsertRequest
	if err := json.Unmarshal(body, &req); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	result, statusCode, err := h.svc.UpsertNamespace(r.Context(), ns, bytes.NewReader(body))
	if err != nil {
		httpapi.WriteError(w, http.StatusBadGateway, "proxy_error", "could not reach api")
		return
	}
	httpapi.WriteJSON(w, statusCode, result)
}

// GetNamespacesOverview handles GET /api/admin/v1/namespaces/overview.
func (h *Handler) GetNamespacesOverview(w http.ResponseWriter, r *http.Request) {
	overview, err := h.svc.GetNamespacesOverview(r.Context())
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "could not build namespace overview")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, overview)
}

// GetBatchRuns handles GET /api/admin/v1/batch-runs.
func (h *Handler) GetBatchRuns(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	ns := q.Get("namespace")
	limit := 20
	if lStr := q.Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}
	if limit > 50 {
		limit = 50
	}

	runs, err := h.svc.GetBatchRuns(r.Context(), ns, limit)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "could not get batch runs")
		return
	}
	if runs == nil {
		runs = []BatchRunLog{}
	}
	httpapi.WriteJSON(w, http.StatusOK, BatchRunsResponse{Runs: runs})
}

// DebugRecommend handles POST /api/admin/v1/recommend/debug.
func (h *Handler) DebugRecommend(w http.ResponseWriter, r *http.Request) {
	var req RecommendDebugRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if req.Namespace == "" || req.SubjectID == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "namespace and subject_id are required")
		return
	}

	result, statusCode, err := h.svc.DebugRecommend(r.Context(), &req)
	if err != nil {
		switch statusCode {
		case http.StatusNotFound:
			httpapi.WriteError(w, http.StatusNotFound, "not_found", "namespace not found")
		case http.StatusUnauthorized:
			httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid api key for namespace")
		case 0:
			httpapi.WriteError(w, http.StatusBadGateway, "proxy_error", "could not reach api")
		default:
			httpapi.WriteError(w, statusCode, "api_error", err.Error())
		}
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, result)
}

// GetQdrantStats handles GET /api/admin/v1/namespaces/{ns}/qdrant-stats.
func (h *Handler) GetQdrantStats(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	stats, err := h.svc.GetQdrantStats(r.Context(), ns)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "could not get qdrant stats")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, stats)
}

// GetSubjectProfile handles GET /api/admin/v1/subjects/{ns}/{id}/profile.
func (h *Handler) GetSubjectProfile(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	id := chi.URLParam(r, "id")

	profile, err := h.svc.GetSubjectProfile(r.Context(), ns, id)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "could not get subject profile")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, profile)
}

// GetTrending handles GET /api/admin/v1/trending/{ns}.
func (h *Handler) GetTrending(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	q := r.URL.Query()

	limit := 50
	if lStr := q.Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}
	offset := 0
	if oStr := q.Get("offset"); oStr != "" {
		if o, err := strconv.Atoi(oStr); err == nil && o >= 0 {
			offset = o
		}
	}
	windowHours := 0
	if whStr := q.Get("window_hours"); whStr != "" {
		if wh, err := strconv.Atoi(whStr); err == nil && wh > 0 {
			windowHours = wh
		}
	}

	result, err := h.svc.GetTrending(r.Context(), ns, limit, offset, windowHours)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadGateway, "proxy_error", "could not get trending data")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, result)
}
