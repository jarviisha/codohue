package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// ─── fake service ─────────────────────────────────────────────────────────────

type fakeSvc struct {
	healthResp    *HealthResponse
	healthStatus  int
	healthErr     error
	nsListResp    []NamespaceConfig
	nsListErr     error
	nsGetResp     *NamespaceConfig
	nsGetErr      error
	upsertResp    *NamespaceUpsertResponse
	upsertStatus  int
	upsertErr     error
	batchRuns     []BatchRunLog
	batchRunsErr  error
	debugResp     *RecommendDebugResponse
	debugStatus   int
	debugErr      error
	trendingResp  *TrendingAdminResponse
	trendingErr   error
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

func (f *fakeSvc) UpsertNamespace(_ context.Context, _ string, _ io.Reader) (*NamespaceUpsertResponse, int, error) {
	return f.upsertResp, f.upsertStatus, f.upsertErr
}

func (f *fakeSvc) GetBatchRuns(_ context.Context, _ string, _ int) ([]BatchRunLog, error) {
	return f.batchRuns, f.batchRunsErr
}

func (f *fakeSvc) DebugRecommend(_ context.Context, _ *RecommendDebugRequest) (*RecommendDebugResponse, int, error) {
	return f.debugResp, f.debugStatus, f.debugErr
}

func (f *fakeSvc) GetTrending(_ context.Context, _ string, _, _, _ int) (*TrendingAdminResponse, error) {
	return f.trendingResp, f.trendingErr
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

func TestLoginSuccess(t *testing.T) {
	h := newTestHandler(&fakeSvc{})
	rec := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/auth/login",
		bytes.NewBufferString(`{"api_key":"test-secret"}`))
	h.Login(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
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

func TestLoginWrongKey(t *testing.T) {
	h := newTestHandler(&fakeSvc{})
	rec := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/auth/login",
		bytes.NewBufferString(`{"api_key":"wrong"}`))
	h.Login(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestLogout(t *testing.T) {
	h := newTestHandler(&fakeSvc{})
	rec := httptest.NewRecorder()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/api/auth/logout", http.NoBody)
	r.AddCookie(sessionCookie(t))
	h.Logout(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
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
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
	var resp HealthResponse
	assertJSON(t, rec, &resp)
	if resp.Status != "degraded" {
		t.Errorf("expected status=degraded, got %q", resp.Status)
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
	if len(resp.Namespaces) != 2 {
		t.Errorf("expected 2 namespaces, got %d", len(resp.Namespaces))
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
	if len(resp.Runs) != 1 {
		t.Errorf("expected 1 run, got %d", len(resp.Runs))
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

// ─── recommend debug handler tests ────────────────────────────────────────────

func TestDebugRecommend_OK(t *testing.T) {
	now := time.Now()
	h := newTestHandler(&fakeSvc{
		debugResp: &RecommendDebugResponse{
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
	body := `{"namespace":"ns1","subject_id":"user-1","limit":10}`
	r := newChiRequest(http.MethodPost, "/api/admin/v1/recommend/debug", nil, body)
	h.DebugRecommend(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
}

func TestDebugRecommend_MissingFields(t *testing.T) {
	h := newTestHandler(&fakeSvc{})
	rec := httptest.NewRecorder()
	body := `{"namespace":"ns1"}` // missing subject_id
	r := newChiRequest(http.MethodPost, "/api/admin/v1/recommend/debug", nil, body)
	h.DebugRecommend(rec, r)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestDebugRecommend_NamespaceNotFound(t *testing.T) {
	h := newTestHandler(&fakeSvc{
		debugStatus: http.StatusNotFound,
		debugErr:    fmt.Errorf("namespace not found"),
	})
	rec := httptest.NewRecorder()
	body := `{"namespace":"unknown","subject_id":"user-1"}`
	r := newChiRequest(http.MethodPost, "/api/admin/v1/recommend/debug", nil, body)
	h.DebugRecommend(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
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
