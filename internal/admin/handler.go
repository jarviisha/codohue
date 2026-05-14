package admin

import (
	"context"
	"encoding/json"
	"errors"
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
	UpsertNamespace(ctx context.Context, namespace string, req *NamespaceUpsertRequest) (*NamespaceUpsertResponse, int, error)
	GetBatchRuns(ctx context.Context, namespace, status, kind string, limit, offset int) ([]BatchRunLog, int, BatchRunStats, error)
	GetSubjectRecommendations(ctx context.Context, namespace, subjectID string, limit, offset int, debug bool) (*RecommendResponse, int, error)
	GetTrending(ctx context.Context, namespace string, limit, offset, windowHours int) (*TrendingAdminResponse, error)
	GetSubjectProfile(ctx context.Context, namespace, subjectID string) (*SubjectProfileResponse, error)
	GetQdrant(ctx context.Context, namespace string) (*QdrantInspectResponse, error)
	CreateBatchRun(ctx context.Context, ns string) (*BatchRunCreateResponse, error)
	GetRecentEvents(ctx context.Context, ns string, limit, offset int, subjectID string) (*EventsListResponse, error)
	InjectEvent(ctx context.Context, ns string, req InjectEventRequest) error
	CreateDemoData(ctx context.Context) (*DemoDatasetResponse, error)
	DeleteDemoData(ctx context.Context) (*DemoDatasetResponse, error)
	GetCatalogConfig(ctx context.Context, namespace string) (*NamespaceCatalogResponse, error)
	UpdateCatalogConfig(ctx context.Context, namespace string, req *NamespaceCatalogUpdateRequest) (*NamespaceCatalogConfig, error)
	TriggerReEmbed(ctx context.Context, namespace string) (*CatalogReEmbedResponse, error)
	ListCatalogItems(ctx context.Context, namespace, state string, limit, offset int, objectIDFilter string) (*CatalogItemsListResponse, error)
	GetCatalogItem(ctx context.Context, namespace string, id int64) (*CatalogItemDetail, error)
	RedriveCatalogItem(ctx context.Context, namespace string, id int64) (*CatalogRedriveResponse, error)
	BulkRedriveDeadletter(ctx context.Context, namespace string) (*CatalogBulkRedriveResponse, error)
	DeleteCatalogItem(ctx context.Context, namespace string, id int64) error
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

// CreateSession handles POST /api/v1/auth/sessions — validates the admin API
// key and issues a session cookie. Returns 201 Created with body
// CreateSessionResponse on success, 401 on bad credentials.
func (h *Handler) CreateSession(w http.ResponseWriter, r *http.Request) {
	var req CreateSessionRequest
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

	expiresAt := time.Now().Add(8 * time.Hour)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/api",
		HttpOnly: true,
		MaxAge:   int((8 * time.Hour).Seconds()),
		SameSite: http.SameSiteLaxMode,
	})
	httpapi.WriteJSON(w, http.StatusCreated, &CreateSessionResponse{ExpiresAt: expiresAt.UTC()})
}

// DeleteCurrentSession handles DELETE /api/v1/auth/sessions/current — clears
// the session cookie and returns 204 No Content.
func (h *Handler) DeleteCurrentSession(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/api",
		HttpOnly: true,
		MaxAge:   -1,
	})
	w.WriteHeader(http.StatusNoContent)
}

// GetHealth handles GET /api/admin/v1/health.
// Always returns HTTP 200 so the frontend can display health state from the JSON body;
// API unreachability is surfaced as status="error" rather than a 4xx/5xx HTTP response.
func (h *Handler) GetHealth(w http.ResponseWriter, r *http.Request) {
	health, _, err := h.svc.GetHealth(r.Context())
	if err != nil {
		httpapi.WriteJSON(w, http.StatusOK, &HealthResponse{
			Postgres: "unknown",
			Redis:    "unknown",
			Qdrant:   "unknown",
			Status:   "error",
		})
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, health)
}

// ListNamespaces handles GET /api/admin/v1/namespaces.
func (h *Handler) ListNamespaces(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("include") == "overview" {
		h.GetNamespacesOverview(w, r)
		return
	}

	namespaces, err := h.svc.ListNamespaces(r.Context())
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "could not list namespaces")
		return
	}
	if namespaces == nil {
		namespaces = []NamespaceConfig{}
	}
	httpapi.WriteJSON(w, http.StatusOK, NamespacesListResponse{Items: namespaces, Total: len(namespaces)})
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

	var req NamespaceUpsertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	result, statusCode, err := h.svc.UpsertNamespace(r.Context(), ns, &req)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "could not upsert namespace")
		return
	}
	httpapi.WriteJSON(w, statusCode, result)
}

// GetCatalogConfig handles GET /api/admin/v1/namespaces/{ns}/catalog.
// Returns 200 with the catalog config + available strategies + backlog
// snapshot, 404 when the namespace does not exist, or 503 when the
// catalog feature is not wired in this deployment.
func (h *Handler) GetCatalogConfig(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	resp, err := h.svc.GetCatalogConfig(r.Context(), ns)
	if err != nil {
		if errors.Is(err, ErrCatalogConfiguratorUnavailable) {
			httpapi.WriteError(w, http.StatusServiceUnavailable, "catalog_unavailable",
				"catalog auto-embedding is not wired in this deployment")
			return
		}
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "could not load catalog config")
		return
	}
	if resp == nil {
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "namespace not found")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, resp)
}

// UpdateCatalogConfig handles PUT /api/admin/v1/namespaces/{ns}/catalog.
// Status code mapping:
//
//	200 OK                       — config applied; body is the new catalog state
//	400 Bad Request              — strategy unknown, or strategy dim mismatch
//	                               (body names both dims)
//	404 Not Found                — namespace does not exist
//	503 Service Unavailable      — catalog adapter not wired
func (h *Handler) UpdateCatalogConfig(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")

	var req NamespaceCatalogUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}

	cfg, err := h.svc.UpdateCatalogConfig(r.Context(), ns, &req)
	if err != nil {
		switch {
		case errors.Is(err, ErrCatalogConfiguratorUnavailable):
			httpapi.WriteError(w, http.StatusServiceUnavailable, "catalog_unavailable",
				"catalog auto-embedding is not wired in this deployment")

		default:
			var dimErr *CatalogDimensionMismatch
			if errors.As(err, &dimErr) {
				// Body must name both dimensions verbatim per
				// contracts/rest-api.md.
				httpapi.WriteJSON(w, http.StatusBadRequest, struct {
					Error                 string `json:"error"`
					StrategyDim           int    `json:"strategy_dim"`
					NamespaceEmbeddingDim int    `json:"namespace_embedding_dim"`
				}{
					Error:                 "strategy dimension mismatch",
					StrategyDim:           dimErr.StrategyDim,
					NamespaceEmbeddingDim: dimErr.NamespaceEmbeddingDim,
				})
				return
			}
			var conflictErr *CatalogStrategyConflict
			if errors.As(err, &conflictErr) {
				httpapi.WriteJSON(w, http.StatusBadRequest, struct {
					Error          string `json:"error"`
					Code           string `json:"code"`
					DenseStrategy  string `json:"dense_strategy"`
					CatalogEnabled bool   `json:"catalog_enabled"`
				}{
					Error:          "dense_strategy must be byoe or disabled when catalog_enabled=true",
					Code:           "dense_strategy_conflict",
					DenseStrategy:  conflictErr.DenseStrategy,
					CatalogEnabled: conflictErr.CatalogEnabled,
				})
				return
			}
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
		}
		return
	}
	if cfg == nil {
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "namespace not found")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, cfg)
}

// GetNamespacesOverview handles GET /api/admin/v1/namespaces?include=overview.
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
	if limit > 100 {
		limit = 100
	}
	offset := 0
	if oStr := q.Get("offset"); oStr != "" {
		if o, err := strconv.Atoi(oStr); err == nil && o >= 0 {
			offset = o
		}
	}

	status := q.Get("status")
	kind := q.Get("kind")
	if kind != "" && kind != "cf" && kind != "reembed" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "kind must be 'cf' or 'reembed'")
		return
	}

	runs, total, stats, err := h.svc.GetBatchRuns(r.Context(), ns, status, kind, limit, offset)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "could not get batch runs")
		return
	}
	if runs == nil {
		runs = []BatchRunLog{}
	}
	httpapi.WriteJSON(w, http.StatusOK, BatchRunsResponse{Items: runs, Total: total, Offset: offset, Stats: stats})
}

// GetSubjectRecommendations handles
// GET /api/admin/v1/namespaces/{ns}/subjects/{id}/recommendations.
// Optional query params: limit, offset, debug. The debug flag enriches the
// response with operator diagnostics.
func (h *Handler) GetSubjectRecommendations(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	id := chi.URLParam(r, "id")
	if ns == "" || id == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "namespace and subject id are required")
		return
	}

	q := r.URL.Query()
	limit := 10
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

	debug := q.Get("debug") == "true"

	result, statusCode, err := h.svc.GetSubjectRecommendations(r.Context(), ns, id, limit, offset, debug)
	if err != nil {
		switch statusCode {
		case http.StatusNotFound:
			httpapi.WriteError(w, http.StatusNotFound, "not_found", "namespace or subject not found")
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

// GetQdrant handles GET /api/admin/v1/namespaces/{ns}/qdrant.
func (h *Handler) GetQdrant(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	stats, err := h.svc.GetQdrant(r.Context(), ns)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "could not get qdrant stats")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, stats)
}

// GetSubjectProfile handles GET /api/admin/v1/namespaces/{ns}/subjects/{id}/profile.
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

// GetTrending handles GET /api/admin/v1/namespaces/{ns}/trending.
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

// CreateBatchRun handles POST /api/admin/v1/namespaces/{ns}/batch-runs.
// Returns 202 Accepted with a Location header pointing to the created run.
func (h *Handler) CreateBatchRun(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")

	result, err := h.svc.CreateBatchRun(r.Context(), ns)
	if err != nil {
		if errors.Is(err, errBatchRunning) {
			httpapi.WriteError(w, http.StatusConflict, "conflict", err.Error())
			return
		}
		if errors.Is(r.Context().Err(), context.DeadlineExceeded) {
			httpapi.WriteError(w, http.StatusGatewayTimeout, "timeout", "batch run timed out")
			return
		}
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "batch trigger failed")
		return
	}
	if result == nil {
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "namespace not found")
		return
	}
	if result.ID > 0 {
		w.Header().Set("Location",
			"/api/admin/v1/namespaces/"+ns+"/batch-runs/"+strconv.FormatInt(result.ID, 10))
	}
	httpapi.WriteJSON(w, http.StatusAccepted, result)
}

// GetRecentEvents handles GET /api/admin/v1/namespaces/{ns}/events.
func (h *Handler) GetRecentEvents(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")
	q := r.URL.Query()

	limit := 50
	if lStr := q.Get("limit"); lStr != "" {
		l, err := strconv.Atoi(lStr)
		if err != nil || l < 1 || l > 200 {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_param", "limit must be between 1 and 200")
			return
		}
		limit = l
	}

	offset := 0
	if oStr := q.Get("offset"); oStr != "" {
		o, err := strconv.Atoi(oStr)
		if err != nil || o < 0 {
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_param", "offset must be a non-negative integer")
			return
		}
		offset = o
	}

	subjectID := q.Get("subject_id")

	result, err := h.svc.GetRecentEvents(r.Context(), ns, limit, offset, subjectID)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "could not fetch events")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, result)
}

// InjectEvent handles POST /api/admin/v1/namespaces/{ns}/events.
func (h *Handler) InjectEvent(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "ns")

	var req InjectEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if req.SubjectID == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "subject_id is required")
		return
	}
	if req.ObjectID == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "object_id is required")
		return
	}

	if err := h.svc.InjectEvent(r.Context(), ns, req); err != nil {
		httpapi.WriteError(w, http.StatusBadGateway, "upstream_error", "upstream event API unavailable")
		return
	}
	httpapi.WriteJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

// CreateDemoData handles POST /api/admin/v1/demo-data — seeds the bundled
// demo namespace and sample events. Returns 202 Accepted with the creation
// summary in the body.
func (h *Handler) CreateDemoData(w http.ResponseWriter, r *http.Request) {
	result, err := h.svc.CreateDemoData(r.Context())
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "could not seed demo dataset")
		return
	}
	httpapi.WriteJSON(w, http.StatusAccepted, result)
}

// DeleteDemoData handles DELETE /api/admin/v1/demo-data — clears the demo
// dataset across postgres, redis, and qdrant. Returns 204 No Content.
func (h *Handler) DeleteDemoData(w http.ResponseWriter, r *http.Request) {
	if _, err := h.svc.DeleteDemoData(r.Context()); err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "could not clear demo dataset")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
