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
	rankNamespace string
	storeErr      error
	deleteErr     error
}

func (f *fakeSvc) Recommend(_ context.Context, _ *Request) (*Response, error) {
	return f.recommendResp, f.recommendErr
}

func (f *fakeSvc) GetTrending(_ context.Context, _ string, _, _, _ int) (*TrendingResponse, error) {
	return f.trendingResp, f.trendingErr
}

func (f *fakeSvc) Rank(_ context.Context, _ *RankRequest, namespace string) (*RankResponse, error) {
	f.rankNamespace = namespace
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

// ─── GET /v1/namespaces/{ns}/subjects/{id}/recommendations ──────────────────

func TestNewHandler(t *testing.T) {
	svc := &Service{}
	h := NewHandler(svc)
	if h == nil || h.service != svc {
		t.Fatal("expected handler to be initialized with provided service")
	}
}

func TestGetSubjectRecommendations_MissingParams(t *testing.T) {
	h := &Handler{}

	for _, params := range []map[string]string{
		{"ns": "ns"}, // missing id
		{"id": "u1"}, // missing ns
		{},           // missing both
	} {
		req := newChiRequest(http.MethodGet,
			"/v1/namespaces/ns/subjects/u1/recommendations", params, "")
		rec := httptest.NewRecorder()
		h.GetSubjectRecommendations(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("params %v: expected 400, got %d", params, rec.Code)
		}
		if got := decodeErrorResponse(t, rec); got.Error.Code != "missing_required_fields" {
			t.Errorf("params %v: unexpected error code %+v", params, got)
		}
	}
}

func TestGetSubjectRecommendations_InvalidLimit(t *testing.T) {
	h := &Handler{}
	for _, q := range []string{"limit=abc", "limit=0", "limit=-1"} {
		req := newChiRequest(http.MethodGet,
			"/v1/namespaces/ns/subjects/u1/recommendations?"+q,
			map[string]string{"ns": "ns", "id": "u1"}, "")
		rec := httptest.NewRecorder()
		h.GetSubjectRecommendations(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("q=%q: expected 400, got %d", q, rec.Code)
		}
	}
}

func TestGetSubjectRecommendations_InvalidOffset(t *testing.T) {
	h := &Handler{}
	req := newChiRequest(http.MethodGet,
		"/v1/namespaces/ns/subjects/u1/recommendations?offset=-1",
		map[string]string{"ns": "ns", "id": "u1"}, "")
	rec := httptest.NewRecorder()
	h.GetSubjectRecommendations(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestGetSubjectRecommendations_Success(t *testing.T) {
	h := &Handler{service: &fakeSvc{
		recommendResp: &Response{
			SubjectID: "u1", Namespace: "ns",
			Items: []RecommendedItem{
				{ObjectID: "item-1", Score: 0.9, Rank: 1},
				{ObjectID: "item-2", Score: 0.7, Rank: 2},
			},
			Source:      SourceFallbackPopular,
			Limit:       20,
			Offset:      0,
			Total:       2,
			GeneratedAt: time.Now(),
		},
	}}

	req := newChiRequest(http.MethodGet,
		"/v1/namespaces/ns/subjects/u1/recommendations",
		map[string]string{"ns": "ns", "id": "u1"}, "")
	rec := httptest.NewRecorder()
	h.GetSubjectRecommendations(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	var resp Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Items) != 2 || resp.Items[0].ObjectID != "item-1" {
		t.Errorf("items: got %v", resp.Items)
	}
}

func TestGetSubjectRecommendations_ServiceError(t *testing.T) {
	h := &Handler{service: &fakeSvc{recommendErr: errors.New("db error")}}
	req := newChiRequest(http.MethodGet,
		"/v1/namespaces/ns/subjects/u1/recommendations",
		map[string]string{"ns": "ns", "id": "u1"}, "")
	rec := httptest.NewRecorder()
	h.GetSubjectRecommendations(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// ─── POST /v1/namespaces/{ns}/rankings ─────────────────────────────────────

func TestRank_MissingNamespacePath(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		"/v1/namespaces//rankings", strings.NewReader(`{"subject_id":"u1","candidates":["p1"]}`))
	rec := httptest.NewRecorder()
	h.Rank(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestRank_MissingSubjectID(t *testing.T) {
	h := &Handler{service: &fakeSvc{}}
	req := newChiRequest(http.MethodPost, "/v1/namespaces/ns/rankings",
		map[string]string{"ns": "ns"}, `{"candidates":["p1"]}`)
	rec := httptest.NewRecorder()
	h.Rank(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestRank_InvalidBody(t *testing.T) {
	h := &Handler{}
	req := newChiRequest(http.MethodPost, "/v1/namespaces/ns/rankings",
		map[string]string{"ns": "ns"}, "not json")
	rec := httptest.NewRecorder()
	h.Rank(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestRank_EmptyCandidates(t *testing.T) {
	h := &Handler{service: &fakeSvc{}}
	req := newChiRequest(http.MethodPost, "/v1/namespaces/ns/rankings",
		map[string]string{"ns": "ns"}, `{"subject_id":"u1","candidates":[]}`)
	rec := httptest.NewRecorder()
	h.Rank(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestRank_TooManyCandidates(t *testing.T) {
	h := &Handler{service: &fakeSvc{}}
	candidates := make([]string, maxCandidates+1)
	for i := range candidates {
		candidates[i] = fmt.Sprintf("item_%d", i)
	}
	body, err := json.Marshal(RankRequest{SubjectID: "u1", Candidates: candidates})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := newChiRequest(http.MethodPost, "/v1/namespaces/ns/rankings",
		map[string]string{"ns": "ns"}, "")
	req.Body = http.NoBody // override; we'll set below via bytes.NewReader
	req = newChiRequest(http.MethodPost, "/v1/namespaces/ns/rankings",
		map[string]string{"ns": "ns"}, "")
	req.Body = httpReader(body)
	rec := httptest.NewRecorder()
	h.Rank(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestRank_Success(t *testing.T) {
	fake := &fakeSvc{
		rankResp: &RankResponse{
			SubjectID: "u1", Namespace: "ns",
			Items:  []RankedItem{{ObjectID: "p1", Score: 0.9, Rank: 1}},
			Source: SourceHybridRank, GeneratedAt: time.Now(),
		},
	}
	h := &Handler{service: fake}

	body, err := json.Marshal(RankRequest{SubjectID: "u1", Candidates: []string{"p1", "p2"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := newChiRequest(http.MethodPost, "/v1/namespaces/ns/rankings",
		map[string]string{"ns": "ns"}, "")
	req.Body = httpReader(body)
	rec := httptest.NewRecorder()
	h.Rank(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if fake.rankNamespace != "ns" {
		t.Errorf("service did not receive path namespace, got %q", fake.rankNamespace)
	}
	var resp RankResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].ObjectID != "p1" {
		t.Errorf("items: got %v", resp.Items)
	}
}

// Body field "namespace" is silently ignored — path is the single source of truth.
func TestRank_BodyNamespaceIgnored(t *testing.T) {
	fake := &fakeSvc{
		rankResp: &RankResponse{
			SubjectID: "u1", Namespace: "ns-from-path",
			Items:  []RankedItem{{ObjectID: "p1", Score: 0.5, Rank: 1}},
			Source: SourceHybridRank, GeneratedAt: time.Now(),
		},
	}
	h := &Handler{service: fake}

	req := newChiRequest(http.MethodPost, "/v1/namespaces/ns-from-path/rankings",
		map[string]string{"ns": "ns-from-path"},
		`{"namespace":"WRONG-WILL-BE-IGNORED","subject_id":"u1","candidates":["p1"]}`)
	rec := httptest.NewRecorder()
	h.Rank(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if fake.rankNamespace != "ns-from-path" {
		t.Errorf("service must receive path namespace, got %q", fake.rankNamespace)
	}
}

func TestRank_ServiceError(t *testing.T) {
	h := &Handler{service: &fakeSvc{rankErr: errors.New("qdrant error")}}
	body, err := json.Marshal(RankRequest{SubjectID: "u1", Candidates: []string{"p1"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := newChiRequest(http.MethodPost, "/v1/namespaces/ns/rankings",
		map[string]string{"ns": "ns"}, "")
	req.Body = httpReader(body)
	rec := httptest.NewRecorder()
	h.Rank(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// ─── GET /v1/namespaces/{ns}/trending ──────────────────────────────────────

func TestGetTrending_MissingNs(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet,
		"/v1/namespaces//trending", http.NoBody)
	rec := httptest.NewRecorder()
	h.GetTrending(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestGetTrending_InvalidLimit(t *testing.T) {
	h := &Handler{}
	req := newChiRequest(http.MethodGet, "/v1/namespaces/ns/trending?limit=0",
		map[string]string{"ns": "ns"}, "")
	rec := httptest.NewRecorder()
	h.GetTrending(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestGetTrending_InvalidOffset(t *testing.T) {
	h := &Handler{}
	req := newChiRequest(http.MethodGet, "/v1/namespaces/ns/trending?offset=-1",
		map[string]string{"ns": "ns"}, "")
	rec := httptest.NewRecorder()
	h.GetTrending(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestGetTrending_Success(t *testing.T) {
	h := &Handler{service: &fakeSvc{
		trendingResp: &TrendingResponse{
			Namespace:   "ns",
			Items:       []TrendingItem{{ObjectID: "item-1", Score: 9.5}},
			WindowHours: 24,
			GeneratedAt: time.Now(),
		},
	}}
	req := newChiRequest(http.MethodGet, "/v1/namespaces/ns/trending",
		map[string]string{"ns": "ns"}, "")
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

// ─── PUT /v1/namespaces/{ns}/{objects|subjects}/{id}/embedding ───────────

func TestStoreEmbedding_MissingVector(t *testing.T) {
	h := &Handler{}
	for _, body := range []string{"", "not-json", `{"vector":[]}`} {
		req := newChiRequest(http.MethodPut, "/v1/namespaces/ns/objects/obj1/embedding",
			map[string]string{"ns": "ns", "id": "obj1"}, body)
		rec := httptest.NewRecorder()
		h.StoreObjectEmbedding(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("body=%q: expected 400, got %d", body, rec.Code)
		}
	}
}

func TestStoreEmbedding_MissingURLParams(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPut,
		"/v1/namespaces/ns/objects//embedding", strings.NewReader(`{"vector":[0.1]}`))
	rec := httptest.NewRecorder()
	h.StoreObjectEmbedding(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestStoreEmbedding_DimMismatch_Returns400(t *testing.T) {
	h := &Handler{
		service: &fakeSvc{storeErr: fmt.Errorf("embedding dimension mismatch: got 128, want 64")},
	}
	req := newChiRequest(http.MethodPut, "/v1/namespaces/ns/objects/obj1/embedding",
		map[string]string{"ns": "ns", "id": "obj1"}, `{"vector":[0.1,0.2]}`)
	rec := httptest.NewRecorder()
	h.StoreObjectEmbedding(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for dim mismatch, got %d", rec.Code)
	}
}

func TestStoreEmbedding_ServiceError_Returns500(t *testing.T) {
	h := &Handler{service: &fakeSvc{storeErr: errors.New("qdrant error")}}
	req := newChiRequest(http.MethodPut, "/v1/namespaces/ns/objects/obj1/embedding",
		map[string]string{"ns": "ns", "id": "obj1"}, `{"vector":[0.1,0.2]}`)
	rec := httptest.NewRecorder()
	h.StoreObjectEmbedding(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestStoreObjectEmbeddingHandler_Success(t *testing.T) {
	h := &Handler{service: &fakeSvc{}}
	req := newChiRequest(http.MethodPut, "/v1/namespaces/ns/objects/obj1/embedding",
		map[string]string{"ns": "ns", "id": "obj1"}, `{"vector":[0.1,0.2,0.3]}`)
	rec := httptest.NewRecorder()
	h.StoreObjectEmbedding(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestStoreSubjectEmbeddingHandler_Success(t *testing.T) {
	h := &Handler{service: &fakeSvc{}}
	req := newChiRequest(http.MethodPut, "/v1/namespaces/ns/subjects/sub1/embedding",
		map[string]string{"ns": "ns", "id": "sub1"}, `{"vector":[0.1,0.2,0.3]}`)
	rec := httptest.NewRecorder()
	h.StoreSubjectEmbedding(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestStoreSubjectEmbeddingHandler_ServiceError(t *testing.T) {
	h := &Handler{service: &fakeSvc{storeErr: errors.New("qdrant error")}}
	req := newChiRequest(http.MethodPut, "/v1/namespaces/ns/subjects/sub1/embedding",
		map[string]string{"ns": "ns", "id": "sub1"}, `{"vector":[0.1,0.2]}`)
	rec := httptest.NewRecorder()
	h.StoreSubjectEmbedding(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// ─── DELETE /v1/namespaces/{ns}/objects/{id} ──────────────────────────────

func TestDeleteObject_MissingParams(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete,
		"/v1/namespaces/ns/objects/", http.NoBody)
	rec := httptest.NewRecorder()
	h.DeleteObject(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestDeleteObject_Success(t *testing.T) {
	h := &Handler{service: &fakeSvc{}}
	req := newChiRequest(http.MethodDelete, "/v1/namespaces/ns/objects/post_1",
		map[string]string{"ns": "ns", "id": "post_1"}, "")
	rec := httptest.NewRecorder()
	h.DeleteObject(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestDeleteObject_ServiceError(t *testing.T) {
	h := &Handler{service: &fakeSvc{deleteErr: errors.New("qdrant error")}}
	req := newChiRequest(http.MethodDelete, "/v1/namespaces/ns/objects/post_1",
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

// httpReader wraps bytes.NewReader so the test can override req.Body with a
// readable + closeable reader after the request has been constructed via the
// newChiRequest helper.
func httpReader(b []byte) interface {
	Read(p []byte) (int, error)
	Close() error
} {
	return readCloser{Reader: bytes.NewReader(b)}
}

type readCloser struct {
	*bytes.Reader
}

func (readCloser) Close() error { return nil }
