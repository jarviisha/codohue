package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// ─── fake service ─────────────────────────────────────────────────────────────

type fakeSvc struct {
	healthResp        *HealthResponse
	healthStatus      int
	healthErr         error
	nsListResp        []NamespaceConfig
	nsListErr         error
	nsGetResp         *NamespaceConfig
	nsGetErr          error
	nsOverviewResp    *NamespacesOverviewResponse
	nsOverviewErr     error
	upsertResp        *NamespaceUpsertResponse
	upsertStatus      int
	upsertErr         error
	batchRuns         []BatchRunLog
	batchRunsErr      error
	batchRunsGotKind  string
	debugResp         *RecommendResponse
	debugStatus       int
	debugErr          error
	trendingResp      *TrendingAdminResponse
	trendingErr       error
	profileResp       *SubjectProfileResponse
	profileErr        error
	qdrantStatsResp   *QdrantInspectResponse
	qdrantStatsErr    error
	triggerResp       *BatchRunCreateResponse
	triggerErr        error
	eventsResp        *EventsListResponse
	eventsErr         error
	injectErr         error
	demoResp          *DemoDatasetResponse
	demoErr           error
	catalogGetResp    *NamespaceCatalogResponse
	catalogGetErr     error
	catalogUpdateResp *NamespaceCatalogConfig
	catalogUpdateErr  error
	catalogUpdateReq  *NamespaceCatalogUpdateRequest

	// US3 operator endpoints
	reembedResp       *CatalogReEmbedResponse
	reembedErr        error
	reembedNS         string
	listItemsResp     *CatalogItemsListResponse
	listItemsErr      error
	listItemsState    string
	listItemsLimit    int
	listItemsOffset   int
	listItemsObjectID string
	getItemResp       *CatalogItemDetail
	getItemErr        error
	getItemID         int64
	redriveResp       *CatalogRedriveResponse
	redriveErr        error
	redriveID         int64
	bulkRedriveResp   *CatalogBulkRedriveResponse
	bulkRedriveErr    error
	bulkRedriveNS     string
	deleteItemErr     error
	deleteItemID      int64
}

func (f *fakeSvc) GetHealth(_ context.Context) (*HealthResponse, int, error) {
	return f.healthResp, f.healthStatus, f.healthErr
}

func (f *fakeSvc) ListNamespaces(_ context.Context) ([]NamespaceConfig, error) {
	return f.nsListResp, f.nsListErr
}

func (f *fakeSvc) GetNamespace(_ context.Context, _ string) (*NamespaceConfig, error) {
	return f.nsGetResp, f.nsGetErr
}

func (f *fakeSvc) GetNamespacesOverview(_ context.Context) (*NamespacesOverviewResponse, error) {
	return f.nsOverviewResp, f.nsOverviewErr
}

func (f *fakeSvc) UpsertNamespace(_ context.Context, _ string, _ *NamespaceUpsertRequest) (*NamespaceUpsertResponse, int, error) {
	return f.upsertResp, f.upsertStatus, f.upsertErr
}

func (f *fakeSvc) GetBatchRuns(_ context.Context, _, _, kind string, _, _ int) ([]BatchRunLog, int, BatchRunStats, error) {
	f.batchRunsGotKind = kind
	return f.batchRuns, len(f.batchRuns), BatchRunStats{Total: len(f.batchRuns)}, f.batchRunsErr
}

func (f *fakeSvc) GetSubjectRecommendations(_ context.Context, _, _ string, _, _ int, _ bool) (*RecommendResponse, int, error) {
	return f.debugResp, f.debugStatus, f.debugErr
}

func (f *fakeSvc) GetTrending(_ context.Context, _ string, _, _, _ int) (*TrendingAdminResponse, error) {
	return f.trendingResp, f.trendingErr
}

func (f *fakeSvc) GetSubjectProfile(_ context.Context, _, _ string) (*SubjectProfileResponse, error) {
	return f.profileResp, f.profileErr
}

func (f *fakeSvc) GetQdrant(_ context.Context, _ string) (*QdrantInspectResponse, error) {
	return f.qdrantStatsResp, f.qdrantStatsErr
}

func (f *fakeSvc) CreateBatchRun(_ context.Context, _ string) (*BatchRunCreateResponse, error) {
	return f.triggerResp, f.triggerErr
}

func (f *fakeSvc) GetRecentEvents(_ context.Context, _ string, _, _ int, _ string) (*EventsListResponse, error) {
	return f.eventsResp, f.eventsErr
}

func (f *fakeSvc) InjectEvent(_ context.Context, _ string, _ InjectEventRequest) error {
	return f.injectErr
}

func (f *fakeSvc) CreateDemoData(_ context.Context) (*DemoDatasetResponse, error) {
	return f.demoResp, f.demoErr
}

func (f *fakeSvc) DeleteDemoData(_ context.Context) (*DemoDatasetResponse, error) {
	return f.demoResp, f.demoErr
}

func (f *fakeSvc) GetCatalogConfig(_ context.Context, _ string) (*NamespaceCatalogResponse, error) {
	return f.catalogGetResp, f.catalogGetErr
}

func (f *fakeSvc) UpdateCatalogConfig(_ context.Context, _ string, req *NamespaceCatalogUpdateRequest) (*NamespaceCatalogConfig, error) {
	f.catalogUpdateReq = req
	return f.catalogUpdateResp, f.catalogUpdateErr
}

func (f *fakeSvc) TriggerReEmbed(_ context.Context, namespace string) (*CatalogReEmbedResponse, error) {
	f.reembedNS = namespace
	return f.reembedResp, f.reembedErr
}

func (f *fakeSvc) ListCatalogItems(_ context.Context, _, state string, limit, offset int, objectIDFilter string) (*CatalogItemsListResponse, error) {
	f.listItemsState = state
	f.listItemsLimit = limit
	f.listItemsOffset = offset
	f.listItemsObjectID = objectIDFilter
	return f.listItemsResp, f.listItemsErr
}

func (f *fakeSvc) GetCatalogItem(_ context.Context, _ string, id int64) (*CatalogItemDetail, error) {
	f.getItemID = id
	return f.getItemResp, f.getItemErr
}

func (f *fakeSvc) RedriveCatalogItem(_ context.Context, _ string, id int64) (*CatalogRedriveResponse, error) {
	f.redriveID = id
	return f.redriveResp, f.redriveErr
}

func (f *fakeSvc) BulkRedriveDeadletter(_ context.Context, namespace string) (*CatalogBulkRedriveResponse, error) {
	f.bulkRedriveNS = namespace
	return f.bulkRedriveResp, f.bulkRedriveErr
}

func (f *fakeSvc) DeleteCatalogItem(_ context.Context, _ string, id int64) error {
	f.deleteItemID = id
	return f.deleteItemErr
}

// ─── helpers ──────────────────────────────────────────────────────────────────

const testAPIKey = "test-secret"

func newTestHandler(svc adminSvc) *Handler {
	return NewHandler(svc, testAPIKey)
}

func newChiRequest(method, target string, urlParams map[string]string, body string) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequestWithContext(context.Background(), method, target, bytes.NewBufferString(body))
	} else {
		r = httptest.NewRequestWithContext(context.Background(), method, target, http.NoBody)
	}
	if len(urlParams) > 0 {
		rctx := chi.NewRouteContext()
		for k, v := range urlParams {
			rctx.URLParams.Add(k, v)
		}
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	}
	return r
}

func sessionCookie(t *testing.T) *http.Cookie {
	t.Helper()
	token, err := createSessionToken(testAPIKey)
	if err != nil {
		t.Fatalf("create session token: %v", err)
	}
	return &http.Cookie{Name: sessionCookieName, Value: token}
}

func assertJSON(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(v); err != nil {
		t.Fatalf("decode response body: %v (body: %s)", err, rec.Body.String())
	}
}

// ─── auth tests ───────────────────────────────────────────────────────────────

func TestCreateSession_Success(t *testing.T) {
	h := newTestHandler(&fakeSvc{})
	rec := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/auth/sessions",
		bytes.NewBufferString(`{"api_key":"test-secret"}`))
	h.CreateSession(rec, r)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie to be set")
	}
	found := false
	for _, c := range cookies {
		if c.Name == sessionCookieName {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("session cookie not found in response")
	}
}

func TestCreateSession_WrongKey(t *testing.T) {
	h := newTestHandler(&fakeSvc{})
	rec := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/auth/sessions",
		bytes.NewBufferString(`{"api_key":"wrong"}`))
	h.CreateSession(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestDeleteCurrentSession(t *testing.T) {
	h := newTestHandler(&fakeSvc{})
	rec := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/api/v1/auth/sessions/current", http.NoBody)
	r.AddCookie(sessionCookie(t))
	h.DeleteCurrentSession(rec, r)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestProtectedRouteWithoutSession(t *testing.T) {
	mw := RequireSession(testAPIKey)
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })
	rec := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/admin/v1/health", http.NoBody)
	mw(next).ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if called {
		t.Fatal("handler should not have been called without a valid session")
	}
}

// ─── health tests ─────────────────────────────────────────────────────────────

func TestGetHealth_OK(t *testing.T) {
	h := newTestHandler(&fakeSvc{
		healthResp:   &HealthResponse{Postgres: "ok", Redis: "ok", Qdrant: "ok", Status: "ok"},
		healthStatus: http.StatusOK,
	})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/health", nil, "")
	r.AddCookie(sessionCookie(t))
	h.GetHealth(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp HealthResponse
	assertJSON(t, rec, &resp)
	if resp.Status != "ok" {
		t.Errorf("expected status=ok, got %q", resp.Status)
	}
}

func TestGetHealth_Degraded(t *testing.T) {
	h := newTestHandler(&fakeSvc{
		healthResp:   &HealthResponse{Postgres: "ok", Redis: "degraded", Qdrant: "ok", Status: "degraded"},
		healthStatus: http.StatusServiceUnavailable,
	})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/health", nil, "")
	h.GetHealth(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp HealthResponse
	assertJSON(t, rec, &resp)
	if resp.Status != "degraded" {
		t.Errorf("expected status=degraded, got %q", resp.Status)
	}
}

func TestGetHealth_Unreachable(t *testing.T) {
	h := newTestHandler(&fakeSvc{
		healthErr: fmt.Errorf("connection refused"),
	})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/health", nil, "")
	h.GetHealth(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp HealthResponse
	assertJSON(t, rec, &resp)
	if resp.Status != "error" {
		t.Errorf("expected status=error, got %q", resp.Status)
	}
	if resp.Postgres != "unknown" || resp.Redis != "unknown" || resp.Qdrant != "unknown" {
		t.Errorf("expected all services unknown, got postgres=%q redis=%q qdrant=%q",
			resp.Postgres, resp.Redis, resp.Qdrant)
	}
}

// ─── namespace handler tests ───────────────────────────────────────────────────

func TestListNamespaces_Handler(t *testing.T) {
	h := newTestHandler(&fakeSvc{
		nsListResp: []NamespaceConfig{{Namespace: "ns1"}, {Namespace: "ns2"}},
	})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/namespaces", nil, "")
	h.ListNamespaces(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp NamespacesListResponse
	assertJSON(t, rec, &resp)
	if len(resp.Items) != 2 || resp.Total != 2 {
		t.Errorf("expected 2 namespaces, got len=%d total=%d", len(resp.Items), resp.Total)
	}
}

func TestGetNamespace_NotFound(t *testing.T) {
	h := newTestHandler(&fakeSvc{nsGetResp: nil})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/namespaces/missing", map[string]string{"ns": "missing"}, "")
	h.GetNamespace(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestUpsertNamespace_NewKey(t *testing.T) {
	key := "plaintext-key"
	h := newTestHandler(&fakeSvc{
		upsertResp:   &NamespaceUpsertResponse{Namespace: "new_ns", UpdatedAt: time.Now(), APIKey: &key},
		upsertStatus: http.StatusOK,
	})
	rec := httptest.NewRecorder()
	body := `{"lambda":0.05,"max_results":50}`
	r := newChiRequest(http.MethodPut, "/api/admin/v1/namespaces/new_ns", map[string]string{"ns": "new_ns"}, body)
	h.UpsertNamespace(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var resp NamespaceUpsertResponse
	assertJSON(t, rec, &resp)
	if resp.APIKey == nil || *resp.APIKey != "plaintext-key" {
		t.Errorf("expected api_key in response for new namespace")
	}
}

func TestUpsertNamespace_ExistingNoKey(t *testing.T) {
	h := newTestHandler(&fakeSvc{
		upsertResp:   &NamespaceUpsertResponse{Namespace: "existing", UpdatedAt: time.Now()},
		upsertStatus: http.StatusOK,
	})
	rec := httptest.NewRecorder()
	body := `{"lambda":0.1}`
	r := newChiRequest(http.MethodPut, "/api/admin/v1/namespaces/existing", map[string]string{"ns": "existing"}, body)
	h.UpsertNamespace(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp NamespaceUpsertResponse
	assertJSON(t, rec, &resp)
	if resp.APIKey != nil {
		t.Errorf("expected no api_key for existing namespace update")
	}
}

// ─── batch runs handler tests ─────────────────────────────────────────────────

func TestGetBatchRuns_All(t *testing.T) {
	now := time.Now()
	h := newTestHandler(&fakeSvc{
		batchRuns: []BatchRunLog{
			{ID: 1, Namespace: "ns1", StartedAt: now, Success: true, SubjectsProcessed: 100},
		},
	})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/batch-runs", nil, "")
	h.GetBatchRuns(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp BatchRunsResponse
	assertJSON(t, rec, &resp)
	if len(resp.Items) != 1 {
		t.Errorf("expected 1 run, got %d", len(resp.Items))
	}
}

func TestGetBatchRuns_FilteredByNamespace(t *testing.T) {
	now := time.Now()
	h := newTestHandler(&fakeSvc{
		batchRuns: []BatchRunLog{
			{ID: 2, Namespace: "filtered_ns", StartedAt: now, Success: true, SubjectsProcessed: 50},
		},
	})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/batch-runs?namespace=filtered_ns", nil, "")
	h.GetBatchRuns(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestGetBatchRuns_LimitCapped(t *testing.T) {
	h := newTestHandler(&fakeSvc{batchRuns: []BatchRunLog{}})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/batch-runs?limit=999", nil, "")
	h.GetBatchRuns(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestGetBatchRuns_KindForwarded(t *testing.T) {
	cases := []struct {
		query    string
		wantKind string
	}{
		{"?kind=cf", "cf"},
		{"?kind=reembed", "reembed"},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			svc := &fakeSvc{batchRuns: []BatchRunLog{}}
			h := newTestHandler(svc)
			rec := httptest.NewRecorder()
			r := newChiRequest(http.MethodGet, "/api/admin/v1/batch-runs"+tc.query, nil, "")
			h.GetBatchRuns(rec, r)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rec.Code)
			}
			if svc.batchRunsGotKind != tc.wantKind {
				t.Errorf("kind = %q, want %q", svc.batchRunsGotKind, tc.wantKind)
			}
		})
	}
}

func TestGetBatchRuns_KindInvalid_400(t *testing.T) {
	h := newTestHandler(&fakeSvc{batchRuns: []BatchRunLog{}})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/batch-runs?kind=bogus", nil, "")
	h.GetBatchRuns(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// ─── recommend debug handler tests ────────────────────────────────────────────

func TestDebugRecommend_OK(t *testing.T) {
	now := time.Now()
	h := newTestHandler(&fakeSvc{
		debugResp: &RecommendResponse{
			SubjectID:   "user-1",
			Namespace:   "ns1",
			Items:       []RecommendDebugItem{{ObjectID: "post_1", Score: 0.9, Rank: 1}},
			Source:      "cf",
			Limit:       10,
			Total:       1,
			GeneratedAt: now,
		},
		debugStatus: http.StatusOK,
	})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet,
		"/api/admin/v1/namespaces/ns1/subjects/user-1/recommendations?limit=10",
		map[string]string{"ns": "ns1", "id": "user-1"}, "")
	h.GetSubjectRecommendations(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestGetSubjectRecommendations_MissingPathParams(t *testing.T) {
	h := newTestHandler(&fakeSvc{})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet,
		"/api/admin/v1/namespaces//subjects//recommendations", nil, "")
	h.GetSubjectRecommendations(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestGetSubjectRecommendations_NamespaceNotFound(t *testing.T) {
	h := newTestHandler(&fakeSvc{
		debugStatus: http.StatusNotFound,
		debugErr:    fmt.Errorf("namespace not found"),
	})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet,
		"/api/admin/v1/namespaces/unknown/subjects/user-1/recommendations",
		map[string]string{"ns": "unknown", "id": "user-1"}, "")
	h.GetSubjectRecommendations(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// ─── subject profile handler tests ───────────────────────────────────────────

func TestGetSubjectProfile_OK(t *testing.T) {
	h := newTestHandler(&fakeSvc{
		profileResp: &SubjectProfileResponse{
			SubjectID:        "user-1",
			Namespace:        "ns1",
			InteractionCount: 5,
			SeenItems:        []string{"post_1", "post_2"},
			SeenItemsDays:    30,
			SparseVectorNNZ:  -1,
		},
	})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/subjects/ns1/user-1/profile",
		map[string]string{"ns": "ns1", "id": "user-1"}, "")
	h.GetSubjectProfile(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var resp SubjectProfileResponse
	assertJSON(t, rec, &resp)
	if resp.InteractionCount != 5 {
		t.Errorf("expected interaction_count=5, got %d", resp.InteractionCount)
	}
}

// ─── qdrant stats handler tests ───────────────────────────────────────────────

func TestGetQdrantStats_OK(t *testing.T) {
	h := newTestHandler(&fakeSvc{
		qdrantStatsResp: &QdrantInspectResponse{
			Subjects:      QdrantCollection{Exists: true, PointsCount: 500},
			Objects:       QdrantCollection{Exists: true, PointsCount: 2000},
			SubjectsDense: QdrantCollection{Exists: false},
			ObjectsDense:  QdrantCollection{Exists: false},
		},
	})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/namespaces/ns1/qdrant",
		map[string]string{"ns": "ns1"}, "")
	h.GetQdrant(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var resp QdrantInspectResponse
	assertJSON(t, rec, &resp)
	if !resp.Subjects.Exists || resp.Subjects.PointsCount != 500 {
		t.Errorf("unexpected subjects stat: %+v", resp.Subjects)
	}
}

// ─── trending handler tests ────────────────────────────────────────────────────

func TestGetTrending_OK(t *testing.T) {
	now := time.Now()
	h := newTestHandler(&fakeSvc{
		trendingResp: &TrendingAdminResponse{
			Namespace:   "ns1",
			Items:       []TrendingAdminEntry{{ObjectID: "post_1", Score: 100.0, CacheTTLSec: 300}},
			WindowHours: 24,
			Limit:       50,
			Total:       1,
			CacheTTLSec: 300,
			GeneratedAt: now,
		},
	})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/trending/ns1", map[string]string{"ns": "ns1"}, "")
	h.GetTrending(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestGetTrending_EmptyCache(t *testing.T) {
	h := newTestHandler(&fakeSvc{
		trendingResp: &TrendingAdminResponse{
			Namespace:   "ns1",
			Items:       []TrendingAdminEntry{},
			CacheTTLSec: -2,
		},
	})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/trending/ns1", map[string]string{"ns": "ns1"}, "")
	h.GetTrending(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp TrendingAdminResponse
	assertJSON(t, rec, &resp)
	if resp.CacheTTLSec != -2 {
		t.Errorf("expected cache_ttl_sec=-2 for empty cache, got %d", resp.CacheTTLSec)
	}
}

// ─── TriggerBatch handler tests ───────────────────────────────────────────────

func TestCreateBatchRun_OK(t *testing.T) {
	svc := &fakeSvc{triggerResp: &BatchRunCreateResponse{ID: 7, Namespace: "ns1", Status: "succeeded", StartedAt: time.Now()}}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodPost, "/api/admin/v1/namespaces/ns1/batch-runs", map[string]string{"ns": "ns1"}, "")
	h.CreateBatchRun(rec, r)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/api/admin/v1/namespaces/ns1/batch-runs/7" {
		t.Errorf("unexpected Location header: %q", loc)
	}
	var resp BatchRunCreateResponse
	assertJSON(t, rec, &resp)
	if resp.ID != 7 {
		t.Errorf("expected id=7, got %d", resp.ID)
	}
}

func TestTriggerBatch_NotFound(t *testing.T) {
	svc := &fakeSvc{triggerResp: nil, triggerErr: nil} // nil,nil → 404
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodPost, "/api/admin/v1/namespaces/missing/batch-runs/trigger", map[string]string{"ns": "missing"}, "")
	h.CreateBatchRun(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestTriggerBatch_Conflict(t *testing.T) {
	svc := &fakeSvc{triggerErr: fmt.Errorf("%w for namespace ns1", errBatchRunning)}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodPost, "/api/admin/v1/namespaces/ns1/batch-runs/trigger", map[string]string{"ns": "ns1"}, "")
	h.CreateBatchRun(rec, r)
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", rec.Code)
	}
}

func TestTriggerBatch_InternalError(t *testing.T) {
	svc := &fakeSvc{triggerErr: fmt.Errorf("db error")}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodPost, "/api/admin/v1/namespaces/ns1/batch-runs/trigger", map[string]string{"ns": "ns1"}, "")
	h.CreateBatchRun(rec, r)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// ─── GetRecentEvents handler tests ───────────────────────────────────────────

func TestGetRecentEvents_OK(t *testing.T) {
	svc := &fakeSvc{eventsResp: &EventsListResponse{
		Items: []EventSummary{{ID: 1, Namespace: "ns1", SubjectID: "user-1", ObjectID: "item-1", Action: "VIEW", Weight: 1.0, OccurredAt: "2026-05-03T10:00:00Z"}},
		Total: 1, Limit: 50, Offset: 0,
	}}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/namespaces/ns1/events", map[string]string{"ns": "ns1"}, "")
	h.GetRecentEvents(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp EventsListResponse
	assertJSON(t, rec, &resp)
	if len(resp.Items) != 1 {
		t.Errorf("expected 1 event, got %d", len(resp.Items))
	}
}

func TestGetRecentEvents_InvalidLimit(t *testing.T) {
	h := newTestHandler(&fakeSvc{})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/namespaces/ns1/events?limit=999", map[string]string{"ns": "ns1"}, "")
	r.URL.RawQuery = "limit=999"
	h.GetRecentEvents(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestGetRecentEvents_InvalidLimitZero(t *testing.T) {
	h := newTestHandler(&fakeSvc{})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/namespaces/ns1/events?limit=0", map[string]string{"ns": "ns1"}, "")
	r.URL.RawQuery = "limit=0"
	h.GetRecentEvents(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestGetRecentEvents_InvalidOffset(t *testing.T) {
	h := newTestHandler(&fakeSvc{})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/namespaces/ns1/events?offset=-1", map[string]string{"ns": "ns1"}, "")
	r.URL.RawQuery = "offset=-1"
	h.GetRecentEvents(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// ─── InjectEvent handler tests ────────────────────────────────────────────────

func TestInjectEvent_OK(t *testing.T) {
	h := newTestHandler(&fakeSvc{injectErr: nil})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodPost, "/api/admin/v1/namespaces/ns1/events",
		map[string]string{"ns": "ns1"},
		`{"subject_id":"user-1","object_id":"item-1","action":"VIEW"}`,
	)
	h.InjectEvent(rec, r)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestInjectEvent_MissingSubjectID(t *testing.T) {
	h := newTestHandler(&fakeSvc{})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodPost, "/api/admin/v1/namespaces/ns1/events",
		map[string]string{"ns": "ns1"},
		`{"subject_id":"","object_id":"item-1","action":"VIEW"}`,
	)
	h.InjectEvent(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestInjectEvent_MissingObjectID(t *testing.T) {
	h := newTestHandler(&fakeSvc{})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodPost, "/api/admin/v1/namespaces/ns1/events",
		map[string]string{"ns": "ns1"},
		`{"subject_id":"user-1","object_id":"","action":"VIEW"}`,
	)
	h.InjectEvent(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestInjectEvent_InvalidJSON(t *testing.T) {
	h := newTestHandler(&fakeSvc{})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodPost, "/api/admin/v1/namespaces/ns1/events",
		map[string]string{"ns": "ns1"},
		`not-json`,
	)
	h.InjectEvent(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestInjectEvent_UpstreamError(t *testing.T) {
	svc := &fakeSvc{injectErr: fmt.Errorf("upstream returned 503")}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodPost, "/api/admin/v1/namespaces/ns1/events",
		map[string]string{"ns": "ns1"},
		`{"subject_id":"user-1","object_id":"item-1","action":"VIEW"}`,
	)
	h.InjectEvent(rec, r)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
}

// ─── Demo dataset handler tests ───────────────────────────────────────────────

func TestCreateDemoData_OK(t *testing.T) {
	h := newTestHandler(&fakeSvc{demoResp: &DemoDatasetResponse{Namespace: "demo", EventsCreated: 25}})
	rec := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/admin/v1/demo-data", http.NoBody)

	h.CreateDemoData(rec, r)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp DemoDatasetResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Namespace != "demo" || resp.EventsCreated != 25 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestDeleteDemoData_OK(t *testing.T) {
	h := newTestHandler(&fakeSvc{demoResp: &DemoDatasetResponse{Namespace: "demo", EventsDeleted: 25}})
	rec := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/api/admin/v1/demo-data", http.NoBody)

	h.DeleteDemoData(rec, r)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── catalog config endpoints (US2) ───────────────────────────────────────────

func TestGetCatalogConfig_OK(t *testing.T) {
	want := &NamespaceCatalogResponse{
		Catalog: NamespaceCatalogConfig{
			Namespace: "ns", Enabled: true,
			StrategyID: "internal-hashing-ngrams", StrategyVersion: "v1",
			EmbeddingDim: 128, MaxAttempts: 5, MaxContentBytes: 32768,
			UpdatedAt: time.Now(),
		},
		AvailableStrategies: []CatalogStrategyDescriptor{
			{ID: "internal-hashing-ngrams", Version: "v1", Dim: 128},
		},
	}
	svc := &fakeSvc{catalogGetResp: want}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/namespaces/ns/catalog", map[string]string{"ns": "ns"}, "")

	h.GetCatalogConfig(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var got NamespaceCatalogResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Catalog.StrategyID != "internal-hashing-ngrams" || got.Catalog.EmbeddingDim != 128 {
		t.Errorf("unexpected catalog body: %+v", got.Catalog)
	}
	if len(got.AvailableStrategies) != 1 {
		t.Errorf("expected 1 available strategy, got %d", len(got.AvailableStrategies))
	}
}

func TestGetCatalogConfig_NotFound(t *testing.T) {
	svc := &fakeSvc{catalogGetResp: nil}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/namespaces/missing/catalog", map[string]string{"ns": "missing"}, "")

	h.GetCatalogConfig(rec, r)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetCatalogConfig_Unavailable(t *testing.T) {
	svc := &fakeSvc{catalogGetErr: ErrCatalogConfiguratorUnavailable}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/namespaces/ns/catalog", map[string]string{"ns": "ns"}, "")

	h.GetCatalogConfig(rec, r)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetCatalogConfig_InternalError(t *testing.T) {
	svc := &fakeSvc{catalogGetErr: fmt.Errorf("db down")}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/namespaces/ns/catalog", map[string]string{"ns": "ns"}, "")

	h.GetCatalogConfig(rec, r)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateCatalogConfig_OK(t *testing.T) {
	want := &NamespaceCatalogConfig{Namespace: "ns", Enabled: true, EmbeddingDim: 128, StrategyID: "internal-hashing-ngrams", StrategyVersion: "v1"}
	svc := &fakeSvc{catalogUpdateResp: want}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	body := `{"enabled":true,"strategy_id":"internal-hashing-ngrams","strategy_version":"v1","params":{"dim":128}}`
	r := newChiRequest(http.MethodPut, "/api/admin/v1/namespaces/ns/catalog", map[string]string{"ns": "ns"}, body)

	h.UpdateCatalogConfig(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if svc.catalogUpdateReq == nil || !svc.catalogUpdateReq.Enabled {
		t.Errorf("expected service to receive enabled=true request, got %+v", svc.catalogUpdateReq)
	}
	if svc.catalogUpdateReq.StrategyID == nil || *svc.catalogUpdateReq.StrategyID != "internal-hashing-ngrams" {
		t.Errorf("strategy_id not propagated: %+v", svc.catalogUpdateReq)
	}
}

func TestUpdateCatalogConfig_DimensionMismatch_400(t *testing.T) {
	svc := &fakeSvc{catalogUpdateErr: &CatalogDimensionMismatch{StrategyDim: 64, NamespaceEmbeddingDim: 128}}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	body := `{"enabled":true,"strategy_id":"x","strategy_version":"v1"}`
	r := newChiRequest(http.MethodPut, "/api/admin/v1/namespaces/ns/catalog", map[string]string{"ns": "ns"}, body)

	h.UpdateCatalogConfig(rec, r)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	// Body must include both dimensions verbatim per contract.
	got := rec.Body.String()
	if !bytes.Contains([]byte(got), []byte(`"strategy_dim":64`)) ||
		!bytes.Contains([]byte(got), []byte(`"namespace_embedding_dim":128`)) {
		t.Errorf("body missing dim fields: %s", got)
	}
}

func TestUpdateCatalogConfig_InvalidJSON_400(t *testing.T) {
	svc := &fakeSvc{}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodPut, "/api/admin/v1/namespaces/ns/catalog", map[string]string{"ns": "ns"}, "{not-json")

	h.UpdateCatalogConfig(rec, r)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateCatalogConfig_NotFound_404(t *testing.T) {
	svc := &fakeSvc{catalogUpdateResp: nil} // nil result + nil error = not found
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	body := `{"enabled":false}`
	r := newChiRequest(http.MethodPut, "/api/admin/v1/namespaces/missing/catalog", map[string]string{"ns": "missing"}, body)

	h.UpdateCatalogConfig(rec, r)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateCatalogConfig_Unavailable_503(t *testing.T) {
	svc := &fakeSvc{catalogUpdateErr: ErrCatalogConfiguratorUnavailable}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	body := `{"enabled":true,"strategy_id":"x","strategy_version":"v1"}`
	r := newChiRequest(http.MethodPut, "/api/admin/v1/namespaces/ns/catalog", map[string]string{"ns": "ns"}, body)

	h.UpdateCatalogConfig(rec, r)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ─── catalog re-embed + items endpoints (US3) ─────────────────────────────────

func TestTriggerReEmbed_Accepted(t *testing.T) {
	want := &CatalogReEmbedResponse{
		BatchRunID: 42, Namespace: "ns",
		StrategyID: "internal-hashing-ngrams", StrategyVersion: "v1",
		StaleItems: 7, StartedAt: time.Now(),
	}
	svc := &fakeSvc{reembedResp: want}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodPost, "/api/admin/v1/namespaces/ns/catalog/re-embed", map[string]string{"ns": "ns"}, "")

	h.TriggerReEmbed(rec, r)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/api/admin/v1/namespaces/ns/batch-runs/42" {
		t.Errorf("unexpected Location: %q", loc)
	}
	var got CatalogReEmbedResponse
	assertJSON(t, rec, &got)
	if got.BatchRunID != 42 || got.StaleItems != 7 {
		t.Errorf("body mismatch: %+v", got)
	}
	if svc.reembedNS != "ns" {
		t.Errorf("expected namespace=ns to reach service, got %q", svc.reembedNS)
	}
}

func TestTriggerReEmbed_NotFound(t *testing.T) {
	svc := &fakeSvc{reembedResp: nil}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodPost, "/api/admin/v1/namespaces/missing/catalog/re-embed", map[string]string{"ns": "missing"}, "")

	h.TriggerReEmbed(rec, r)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTriggerReEmbed_Conflict_409(t *testing.T) {
	svc := &fakeSvc{reembedErr: ErrReembedAlreadyRunning}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodPost, "/api/admin/v1/namespaces/ns/catalog/re-embed", map[string]string{"ns": "ns"}, "")

	h.TriggerReEmbed(rec, r)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestTriggerReEmbed_Unavailable_503(t *testing.T) {
	svc := &fakeSvc{reembedErr: ErrCatalogStrategyPickerUnavailable}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodPost, "/api/admin/v1/namespaces/ns/catalog/re-embed", map[string]string{"ns": "ns"}, "")

	h.TriggerReEmbed(rec, r)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListCatalogItems_OK(t *testing.T) {
	want := &CatalogItemsListResponse{
		Items: []CatalogItemSummary{
			{ID: 1, ObjectID: "o1", State: "embedded", AttemptCount: 1, UpdatedAt: time.Now()},
		},
		Total: 1, Limit: 50, Offset: 0,
	}
	svc := &fakeSvc{listItemsResp: want}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/namespaces/ns/catalog/items?state=embedded&limit=20&offset=10&object_id=foo", map[string]string{"ns": "ns"}, "")

	h.ListCatalogItems(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if svc.listItemsState != "embedded" || svc.listItemsLimit != 20 || svc.listItemsOffset != 10 || svc.listItemsObjectID != "foo" {
		t.Errorf("query params not propagated: state=%q limit=%d offset=%d object_id=%q",
			svc.listItemsState, svc.listItemsLimit, svc.listItemsOffset, svc.listItemsObjectID)
	}
}

func TestListCatalogItems_DefaultsState(t *testing.T) {
	svc := &fakeSvc{listItemsResp: &CatalogItemsListResponse{Items: []CatalogItemSummary{}}}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/namespaces/ns/catalog/items", map[string]string{"ns": "ns"}, "")

	h.ListCatalogItems(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if svc.listItemsState != "all" {
		t.Errorf("expected state default 'all', got %q", svc.listItemsState)
	}
}

func TestListCatalogItems_BadLimit(t *testing.T) {
	h := newTestHandler(&fakeSvc{})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/namespaces/ns/catalog/items?limit=99999", map[string]string{"ns": "ns"}, "")

	h.ListCatalogItems(rec, r)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestGetCatalogItem_OK(t *testing.T) {
	want := &CatalogItemDetail{
		CatalogItemSummary: CatalogItemSummary{ID: 7, ObjectID: "o7", State: "embedded"},
		Namespace:          "ns",
		Content:            "hello",
		Metadata:           map[string]any{"author_id": "u1"},
	}
	svc := &fakeSvc{getItemResp: want}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/namespaces/ns/catalog/items/7",
		map[string]string{"ns": "ns", "id": "7"}, "")

	h.GetCatalogItem(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if svc.getItemID != 7 {
		t.Errorf("expected id=7, got %d", svc.getItemID)
	}
}

func TestGetCatalogItem_NotFound(t *testing.T) {
	svc := &fakeSvc{getItemResp: nil}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/namespaces/ns/catalog/items/999",
		map[string]string{"ns": "ns", "id": "999"}, "")

	h.GetCatalogItem(rec, r)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestGetCatalogItem_BadID(t *testing.T) {
	h := newTestHandler(&fakeSvc{})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodGet, "/api/admin/v1/namespaces/ns/catalog/items/notanint",
		map[string]string{"ns": "ns", "id": "notanint"}, "")

	h.GetCatalogItem(rec, r)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestRedriveCatalogItem_Accepted(t *testing.T) {
	want := &CatalogRedriveResponse{ID: 5, ObjectID: "o5", State: "pending"}
	svc := &fakeSvc{redriveResp: want}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodPost, "/api/admin/v1/namespaces/ns/catalog/items/5/redrive",
		map[string]string{"ns": "ns", "id": "5"}, "")

	h.RedriveCatalogItem(rec, r)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if svc.redriveID != 5 {
		t.Errorf("expected id=5, got %d", svc.redriveID)
	}
}

func TestRedriveCatalogItem_NotFound(t *testing.T) {
	svc := &fakeSvc{redriveResp: nil}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodPost, "/api/admin/v1/namespaces/ns/catalog/items/5/redrive",
		map[string]string{"ns": "ns", "id": "5"}, "")

	h.RedriveCatalogItem(rec, r)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestRedriveCatalogItem_Unavailable_503(t *testing.T) {
	svc := &fakeSvc{redriveErr: ErrCatalogStrategyPickerUnavailable}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodPost, "/api/admin/v1/namespaces/ns/catalog/items/5/redrive",
		map[string]string{"ns": "ns", "id": "5"}, "")

	h.RedriveCatalogItem(rec, r)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestBulkRedriveDeadletter_OK(t *testing.T) {
	want := &CatalogBulkRedriveResponse{Namespace: "ns", Redriven: 3}
	svc := &fakeSvc{bulkRedriveResp: want}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodPost, "/api/admin/v1/namespaces/ns/catalog/items/redrive-deadletter",
		map[string]string{"ns": "ns"}, "")

	h.BulkRedriveDeadletter(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if svc.bulkRedriveNS != "ns" {
		t.Errorf("expected namespace=ns to reach service, got %q", svc.bulkRedriveNS)
	}
	var got CatalogBulkRedriveResponse
	assertJSON(t, rec, &got)
	if got.Redriven != 3 {
		t.Errorf("expected redriven=3, got %d", got.Redriven)
	}
}

func TestBulkRedriveDeadletter_NotFound(t *testing.T) {
	svc := &fakeSvc{bulkRedriveResp: nil}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodPost, "/api/admin/v1/namespaces/missing/catalog/items/redrive-deadletter",
		map[string]string{"ns": "missing"}, "")

	h.BulkRedriveDeadletter(rec, r)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestDeleteCatalogItem_NoContent(t *testing.T) {
	svc := &fakeSvc{}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodDelete, "/api/admin/v1/namespaces/ns/catalog/items/9",
		map[string]string{"ns": "ns", "id": "9"}, "")

	h.DeleteCatalogItem(rec, r)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}
	if svc.deleteItemID != 9 {
		t.Errorf("expected id=9, got %d", svc.deleteItemID)
	}
}

func TestDeleteCatalogItem_BadID(t *testing.T) {
	h := newTestHandler(&fakeSvc{})
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodDelete, "/api/admin/v1/namespaces/ns/catalog/items/-1",
		map[string]string{"ns": "ns", "id": "-1"}, "")

	h.DeleteCatalogItem(rec, r)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestDeleteCatalogItem_InternalError(t *testing.T) {
	svc := &fakeSvc{deleteItemErr: fmt.Errorf("db down")}
	h := newTestHandler(svc)
	rec := httptest.NewRecorder()
	r := newChiRequest(http.MethodDelete, "/api/admin/v1/namespaces/ns/catalog/items/9",
		map[string]string{"ns": "ns", "id": "9"}, "")

	h.DeleteCatalogItem(rec, r)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
