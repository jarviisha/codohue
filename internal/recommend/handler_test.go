package recommend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jarviisha/codohue/internal/core/httpapi"
)

// ─── fake service ────────────────────────────────────────────────────────────

type fakeSvc struct {
	recommendResp *Response
	recommendErr  error
	trendingResp  *TrendingResponse
	trendingErr   error
	rankResp      *RankResponse
	rankErr       error
	storeErr      error
	deleteErr     error
}

func (f *fakeSvc) Recommend(_ context.Context, _ *Request) (*Response, error) {
	return f.recommendResp, f.recommendErr
}

func (f *fakeSvc) GetTrending(_ context.Context, _ string, _, _, _ int) (*TrendingResponse, error) {
	return f.trendingResp, f.trendingErr
}

func (f *fakeSvc) Rank(_ context.Context, _ *RankRequest) (*RankResponse, error) {
	return f.rankResp, f.rankErr
}

func (f *fakeSvc) StoreObjectEmbedding(_ context.Context, _, _ string, _ []float32) error {
	return f.storeErr
}

func (f *fakeSvc) StoreSubjectEmbedding(_ context.Context, _, _ string, _ []float32) error {
	return f.storeErr
}

func (f *fakeSvc) DeleteObject(_ context.Context, _, _ string) error {
	return f.deleteErr
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func newChiRequest(method, target string, params map[string]string, body string) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequestWithContext(context.Background(), method, target, strings.NewReader(body))
	} else {
		r = httptest.NewRequestWithContext(context.Background(), method, target, http.NoBody)
	}
	if len(params) > 0 {
		rctx := chi.NewRouteContext()
		for k, v := range params {
			rctx.URLParams.Add(k, v)
		}
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
	}
	return r
}

func decodeErrorResponse(t *testing.T, rec *httptest.ResponseRecorder) httpapi.ErrorResponse {
	t.Helper()
	var resp httpapi.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	return resp
}

// ─── GET /v1/recommendations ─────────────────────────────────────────────────

func TestHandlerGetMissingParams(t *testing.T) {
	h := &Handler{}

	for _, url := range []string{
		"/v1/recommendations?namespace=test",
		"/v1/recommendations?subject_id=u1",
		"/v1/recommendations",
	} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
		rec := httptest.NewRecorder()
		h.Get(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("url %s: expected 400, got %d", url, rec.Code)
		}
	}
}

func TestHandlerGetMissingParams_JSONError(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/recommendations", http.NoBody)
	rec := httptest.NewRecorder()

	h.Get(rec, req)

	if got := decodeErrorResponse(t, rec); got.Error.Code != "missing_required_fields" {
		t.Fatalf("unexpected error code: %+v", got)
	}
}

func TestNewHandler(t *testing.T) {
	svc := &Service{}
	validate := func(_ context.Context, _, _ string) bool { return true }
	h := NewHandler(svc, validate)
	if h == nil || h.service != svc {
		t.Fatal("expected handler to be initialized with provided service")
	}
}

func TestHandlerGetInvalidLimit(t *testing.T) {
	h := &Handler{}

	for _, url := range []string{
		"/v1/recommendations?subject_id=u1&namespace=ns&limit=abc",
		"/v1/recommendations?subject_id=u1&namespace=ns&limit=0",
		"/v1/recommendations?subject_id=u1&namespace=ns&limit=-1",
	} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, url, http.NoBody)
		rec := httptest.NewRecorder()
		h.Get(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("url %s: expected 400, got %d", url, rec.Code)
		}
	}
}

func TestHandlerGetSuccess(t *testing.T) {
	h := &Handler{service: &fakeSvc{
		recommendResp: &Response{
			SubjectID: "u1", Namespace: "ns",
			Items: []string{"item-1", "item-2"}, Source: SourceFallbackPopular,
			GeneratedAt: time.Now(),
		},
	}}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet,
		"/v1/recommendations?subject_id=u1&namespace=ns", http.NoBody)
	rec := httptest.NewRecorder()
	h.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var resp Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Items) != 2 || resp.Items[0] != "item-1" {
		t.Errorf("items: got %v", resp.Items)
	}
}

func TestHandlerGetServiceError(t *testing.T) {
	h := &Handler{service: &fakeSvc{recommendErr: errors.New("db error")}}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet,
		"/v1/recommendations?subject_id=u1&namespace=ns", http.NoBody)
	rec := httptest.NewRecorder()
	h.Get(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// ─── POST /v1/rank ───────────────────────────────────────────────────────────

func TestHandlerRankMissingFields(t *testing.T) {
	h := &Handler{}

	for _, body := range []string{
		`{"namespace":"ns","candidates":["p1"]}`,
		`{"subject_id":"u1","candidates":["p1"]}`,
		`{"candidates":["p1"]}`,
	} {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/rank", strings.NewReader(body))
		rec := httptest.NewRecorder()
		h.Rank(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: expected 400, got %d", body, rec.Code)
		}
	}
}

func TestHandlerRankInvalidBody(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/rank", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	h.Rank(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandlerRankMaxCandidates(t *testing.T) {
	h := &Handler{}
	candidates := make([]string, maxCandidates+1)
	for i := range candidates {
		candidates[i] = fmt.Sprintf("item_%d", i)
	}
	body, err := json.Marshal(RankRequest{SubjectID: "u1", Namespace: "ns", Candidates: candidates})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/rank", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.Rank(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestHandlerRankUnauthorized(t *testing.T) {
	h := &Handler{
		service:     &fakeSvc{},
		validateKey: func(_ context.Context, _, _ string) bool { return false },
	}
	body, err := json.Marshal(RankRequest{SubjectID: "u1", Namespace: "ns", Candidates: []string{"p1"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/rank", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	h.Rank(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestHandlerRankSuccess(t *testing.T) {
	h := &Handler{
		service: &fakeSvc{
			rankResp: &RankResponse{
				SubjectID: "u1", Namespace: "ns",
				Items:  []RankedItem{{ObjectID: "p1", Score: 0.9}},
				Source: SourceHybridRank, GeneratedAt: time.Now(),
			},
		},
		validateKey: func(_ context.Context, _, _ string) bool { return true },
	}
	body, err := json.Marshal(RankRequest{SubjectID: "u1", Namespace: "ns", Candidates: []string{"p1", "p2"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/rank", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()
	h.Rank(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var resp RankResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].ObjectID != "p1" {
		t.Errorf("items: got %v", resp.Items)
	}
}

func TestHandlerRankByNamespaceSuccess(t *testing.T) {
	h := &Handler{
		service: &fakeSvc{
			rankResp: &RankResponse{
				SubjectID: "u1", Namespace: "ns",
				Items:  []RankedItem{{ObjectID: "p1", Score: 0.9}},
				Source: SourceHybridRank, GeneratedAt: time.Now(),
			},
		},
		validateKey: func(_ context.Context, _, _ string) bool { return true },
	}
	req := newChiRequest(http.MethodPost, "/v1/namespaces/ns/rank", map[string]string{"ns": "ns"}, `{"subject_id":"u1","candidates":["p1"]}`)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()

	h.RankByNamespace(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp RankResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Namespace != "ns" {
		t.Fatalf("namespace: got %q", resp.Namespace)
	}
}

func TestHandlerRankByNamespaceMismatch(t *testing.T) {
	h := &Handler{}
	req := newChiRequest(http.MethodPost, "/v1/namespaces/ns-a/rank", map[string]string{"ns": "ns-a"}, `{"namespace":"ns-b","subject_id":"u1","candidates":["p1"]}`)
	rec := httptest.NewRecorder()

	h.RankByNamespace(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if got := decodeErrorResponse(t, rec); got.Error.Code != "namespace_mismatch" {
		t.Fatalf("unexpected error code: %+v", got)
	}
}

func TestHandlerRankServiceError(t *testing.T) {
	h := &Handler{service: &fakeSvc{rankErr: errors.New("qdrant error")}}
	body, err := json.Marshal(RankRequest{SubjectID: "u1", Namespace: "ns", Candidates: []string{"p1"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/rank", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.Rank(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// ─── GET /v1/trending/{ns} ───────────────────────────────────────────────────

func TestGetTrendingMissingNs(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/trending/", http.NoBody)
	rec := httptest.NewRecorder()
	h.GetTrending(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestGetTrendingInvalidLimit(t *testing.T) {
	h := &Handler{}
	req := newChiRequest(http.MethodGet, "/v1/trending/ns?limit=0", map[string]string{"ns": "ns"}, "")
	rec := httptest.NewRecorder()
	h.GetTrending(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestGetTrendingInvalidOffset(t *testing.T) {
	h := &Handler{}
	req := newChiRequest(http.MethodGet, "/v1/trending/ns?offset=-1", map[string]string{"ns": "ns"}, "")
	rec := httptest.NewRecorder()
	h.GetTrending(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestGetTrendingSuccess(t *testing.T) {
	h := &Handler{service: &fakeSvc{
		trendingResp: &TrendingResponse{
			Namespace:   "ns",
			Items:       []TrendingItem{{ObjectID: "item-1", Score: 9.5}},
			WindowHours: 24,
			GeneratedAt: time.Now(),
		},
	}}
	req := newChiRequest(http.MethodGet, "/v1/trending/ns", map[string]string{"ns": "ns"}, "")
	rec := httptest.NewRecorder()
	h.GetTrending(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var resp TrendingResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].ObjectID != "item-1" {
		t.Errorf("items: %v", resp.Items)
	}
}

// ─── POST /v1/{objects|subjects}/{ns}/{id}/embedding ─────────────────────────

func TestStoreEmbeddingMissingVector(t *testing.T) {
	h := &Handler{}
	for _, body := range []string{"", "not-json", `{"vector":[]}`} {
		req := newChiRequest(http.MethodPost, "/v1/objects/ns/obj1/embedding",
			map[string]string{"ns": "ns", "id": "obj1"}, body)
		rec := httptest.NewRecorder()
		h.StoreObjectEmbedding(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body=%q: expected 400, got %d", body, rec.Code)
		}
	}
}

func TestStoreEmbeddingUnauthorized(t *testing.T) {
	h := &Handler{
		service:     &fakeSvc{},
		validateKey: func(_ context.Context, _, _ string) bool { return false },
	}
	req := newChiRequest(http.MethodPost, "/v1/objects/ns/obj1/embedding",
		map[string]string{"ns": "ns", "id": "obj1"}, `{"vector":[0.1,0.2]}`)
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	h.StoreObjectEmbedding(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestStoreEmbeddingMissingURLParams(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		"/v1/objects//embedding", strings.NewReader(`{"vector":[0.1]}`))
	rec := httptest.NewRecorder()
	h.StoreObjectEmbedding(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestStoreEmbeddingDimMismatch_Returns400(t *testing.T) {
	h := &Handler{
		service:     &fakeSvc{storeErr: fmt.Errorf("embedding dimension mismatch: got 128, want 64")},
		validateKey: func(_ context.Context, _, _ string) bool { return true },
	}
	req := newChiRequest(http.MethodPost, "/v1/objects/ns/obj1/embedding",
		map[string]string{"ns": "ns", "id": "obj1"}, `{"vector":[0.1,0.2]}`)
	rec := httptest.NewRecorder()
	h.StoreObjectEmbedding(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for dim mismatch, got %d", rec.Code)
	}
}

func TestStoreEmbeddingServiceError_Returns500(t *testing.T) {
	h := &Handler{
		service:     &fakeSvc{storeErr: errors.New("qdrant error")},
		validateKey: func(_ context.Context, _, _ string) bool { return true },
	}
	req := newChiRequest(http.MethodPost, "/v1/objects/ns/obj1/embedding",
		map[string]string{"ns": "ns", "id": "obj1"}, `{"vector":[0.1,0.2]}`)
	rec := httptest.NewRecorder()
	h.StoreObjectEmbedding(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestStoreEmbeddingSuccess(t *testing.T) {
	h := &Handler{
		service:     &fakeSvc{storeErr: nil},
		validateKey: func(_ context.Context, _, _ string) bool { return true },
	}
	req := newChiRequest(http.MethodPost, "/v1/objects/ns/obj1/embedding",
		map[string]string{"ns": "ns", "id": "obj1"}, `{"vector":[0.1,0.2,0.3]}`)
	rec := httptest.NewRecorder()
	h.StoreObjectEmbedding(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestStoreSubjectEmbeddingSuccess(t *testing.T) {
	h := &Handler{
		service:     &fakeSvc{storeErr: nil},
		validateKey: func(_ context.Context, _, _ string) bool { return true },
	}
	req := newChiRequest(http.MethodPost, "/v1/subjects/ns/sub1/embedding",
		map[string]string{"ns": "ns", "id": "sub1"}, `{"vector":[0.1,0.2,0.3]}`)
	rec := httptest.NewRecorder()
	h.StoreSubjectEmbedding(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestStoreSubjectEmbeddingServiceError(t *testing.T) {
	h := &Handler{
		service:     &fakeSvc{storeErr: errors.New("qdrant error")},
		validateKey: func(_ context.Context, _, _ string) bool { return true },
	}
	req := newChiRequest(http.MethodPost, "/v1/subjects/ns/sub1/embedding",
		map[string]string{"ns": "ns", "id": "sub1"}, `{"vector":[0.1,0.2]}`)
	rec := httptest.NewRecorder()
	h.StoreSubjectEmbedding(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// ─── DELETE /v1/objects/{ns}/{id} ────────────────────────────────────────────

func TestDeleteObjectMissingParams(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete,
		"/v1/objects//obj1", http.NoBody)
	rec := httptest.NewRecorder()
	h.DeleteObject(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestDeleteObjectUnauthorized(t *testing.T) {
	h := &Handler{
		service:     &fakeSvc{},
		validateKey: func(_ context.Context, _, _ string) bool { return false },
	}
	req := newChiRequest(http.MethodDelete, "/v1/objects/ns/post_1",
		map[string]string{"ns": "ns", "id": "post_1"}, "")
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	h.DeleteObject(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestDeleteObjectSuccess(t *testing.T) {
	h := &Handler{
		service:     &fakeSvc{deleteErr: nil},
		validateKey: func(_ context.Context, _, _ string) bool { return true },
	}
	req := newChiRequest(http.MethodDelete, "/v1/objects/ns/post_1",
		map[string]string{"ns": "ns", "id": "post_1"}, "")
	rec := httptest.NewRecorder()
	h.DeleteObject(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestDeleteObjectServiceError(t *testing.T) {
	h := &Handler{
		service:     &fakeSvc{deleteErr: errors.New("qdrant error")},
		validateKey: func(_ context.Context, _, _ string) bool { return true },
	}
	req := newChiRequest(http.MethodDelete, "/v1/objects/ns/post_1",
		map[string]string{"ns": "ns", "id": "post_1"}, "")
	rec := httptest.NewRecorder()
	h.DeleteObject(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// ─── misc ─────────────────────────────────────────────────────────────────────

func TestIsDimMismatch(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmt.Errorf("embedding dimension mismatch: got 128, want 64"), true},
		{fmt.Errorf("some other error"), false},
	}
	for _, tt := range cases {
		if got := isDimMismatch(tt.err); got != tt.want {
			t.Errorf("isDimMismatch(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}
