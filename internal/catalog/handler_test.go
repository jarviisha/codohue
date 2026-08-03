package catalog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

type fakeIngester struct {
	item *Item
	err  error

	lastNS  string
	lastReq *IngestRequest

	batchResp *codohuetypes.CatalogBatchIngestResponse
	batchErr  error
	lastBatch *BatchIngestRequest

	listResp  *codohuetypes.CatalogObjectsResponse
	listErr   error
	lastSince *time.Time
	lastLimit int
}

func (f *fakeIngester) Ingest(_ context.Context, ns string, req *IngestRequest) (*Item, error) {
	f.lastNS = ns
	f.lastReq = req
	return f.item, f.err
}

func (f *fakeIngester) IngestBatch(_ context.Context, ns string, req *BatchIngestRequest) (*codohuetypes.CatalogBatchIngestResponse, error) {
	f.lastNS = ns
	f.lastBatch = req
	return f.batchResp, f.batchErr
}

func (f *fakeIngester) ListObjects(_ context.Context, ns string, changedSince *time.Time, limit, _ int) (*codohuetypes.CatalogObjectsResponse, error) {
	f.lastNS = ns
	f.lastSince = changedSince
	f.lastLimit = limit
	return f.listResp, f.listErr
}

func newCatalogRequest(body, namespace string) *http.Request {
	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/v1/namespaces/"+namespace+"/catalog",
		strings.NewReader(body),
	)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("ns", namespace)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestHandlerIngest_HappyPath_202(t *testing.T) {
	h := &Handler{service: &fakeIngester{item: &Item{ID: 1, Namespace: "ns", ObjectID: "o1"}}}
	rec := httptest.NewRecorder()
	h.Ingest(rec, newCatalogRequest(`{"object_id":"o1","content":"hello"}`, "ns"))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body on 202, got %q", rec.Body.String())
	}
}

func TestHandlerIngest_BodyNamespaceRejected(t *testing.T) {
	// "namespace" is not part of the catalog ingest contract — the URL path is
	// the only source of truth. A body that sneaks one in is rejected as an
	// unknown field (400) instead of being silently ignored, so the service is
	// never reached.
	ing := &fakeIngester{item: &Item{ID: 1}}
	h := &Handler{service: ing}
	rec := httptest.NewRecorder()
	h.Ingest(rec, newCatalogRequest(`{"object_id":"o1","content":"hi","namespace":"WRONG"}`, "real-ns"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if ing.lastNS != "" {
		t.Errorf("service must not be called on a rejected body, got ns %q", ing.lastNS)
	}
}

func TestHandlerIngest_MissingNamespaceURLParam_400(t *testing.T) {
	h := &Handler{service: &fakeIngester{}}
	// A request with no chi URL param results in chi.URLParam returning "".
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/x", strings.NewReader(`{"object_id":"o1","content":"hi"}`))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("ns", "")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	h.Ingest(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandlerIngest_InvalidJSON_400(t *testing.T) {
	h := &Handler{service: &fakeIngester{}}
	rec := httptest.NewRecorder()
	h.Ingest(rec, newCatalogRequest(`{not-json`, "ns"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandlerIngest_ServiceErrInvalidRequest_400(t *testing.T) {
	h := &Handler{service: &fakeIngester{err: ErrInvalidRequest}}
	rec := httptest.NewRecorder()
	h.Ingest(rec, newCatalogRequest(`{"object_id":"","content":"hi"}`, "ns"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandlerIngest_ServiceErrEmptyContent_422(t *testing.T) {
	h := &Handler{service: &fakeIngester{err: ErrEmptyContent}}
	rec := httptest.NewRecorder()
	h.Ingest(rec, newCatalogRequest(`{"object_id":"o1","content":"   "}`, "ns"))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", rec.Code)
	}
}

func TestHandlerIngest_ServiceErrContentTooLarge_413(t *testing.T) {
	h := &Handler{service: &fakeIngester{err: ErrContentTooLarge}}
	rec := httptest.NewRecorder()
	h.Ingest(rec, newCatalogRequest(`{"object_id":"o1","content":"big"}`, "ns"))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", rec.Code)
	}
}

func TestHandlerIngest_ServiceErrNamespaceNotEnabled_404(t *testing.T) {
	h := &Handler{service: &fakeIngester{err: ErrNamespaceNotEnabled}}
	rec := httptest.NewRecorder()
	h.Ingest(rec, newCatalogRequest(`{"object_id":"o1","content":"hi"}`, "ns"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "namespace_not_enabled") {
		t.Errorf("expected error code namespace_not_enabled, got %s", rec.Body.String())
	}
}

func TestHandlerIngest_ServiceErrNamespaceNotFound_404SameBody(t *testing.T) {
	h := &Handler{service: &fakeIngester{err: ErrNamespaceNotFound}}
	rec := httptest.NewRecorder()
	h.Ingest(rec, newCatalogRequest(`{"object_id":"o1","content":"hi"}`, "ns"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	// The body must be IDENTICAL to the not-enabled case so unauthenticated
	// probes cannot enumerate namespaces.
	if !strings.Contains(rec.Body.String(), "namespace not found or catalog auto-embedding not enabled") {
		t.Errorf("expected unified body, got %s", rec.Body.String())
	}
}

func TestHandlerIngest_UnknownServiceError_500(t *testing.T) {
	h := &Handler{service: &fakeIngester{err: errors.New("kaboom")}}
	rec := httptest.NewRecorder()
	h.Ingest(rec, newCatalogRequest(`{"object_id":"o1","content":"hi"}`, "ns"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "internal_error") {
		t.Errorf("expected error code internal_error, got %s", rec.Body.String())
	}
}

func newCatalogPathRequest(method, path, body, namespace string) *http.Request {
	req := httptest.NewRequestWithContext(
		context.Background(),
		method,
		"/v1/namespaces/"+namespace+path,
		strings.NewReader(body),
	)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("ns", namespace)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestHandlerBatchIngest_ReturnsPerItemResults(t *testing.T) {
	fake := &fakeIngester{batchResp: &codohuetypes.CatalogBatchIngestResponse{
		Namespace: "ns", Accepted: 1, Rejected: 1,
		Results: []codohuetypes.CatalogBatchItemResult{
			{ObjectID: "o1", Accepted: true},
			{ObjectID: "o2", Accepted: false, Error: "empty_content"},
		},
	}}
	h := &Handler{service: fake}
	rec := httptest.NewRecorder()
	h.BatchIngest(rec, newCatalogPathRequest(http.MethodPost, "/catalog/batch",
		`{"items":[{"object_id":"o1","content":"hi"},{"object_id":"o2","content":" "}]}`, "ns"))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.lastBatch == nil || len(fake.lastBatch.Items) != 2 {
		t.Fatalf("service did not receive the batch: %+v", fake.lastBatch)
	}
	if !strings.Contains(rec.Body.String(), `"empty_content"`) {
		t.Errorf("per-item error missing from body: %s", rec.Body.String())
	}
}

func TestHandlerBatchIngest_InvalidBody_400(t *testing.T) {
	h := &Handler{service: &fakeIngester{}}
	rec := httptest.NewRecorder()
	h.BatchIngest(rec, newCatalogPathRequest(http.MethodPost, "/catalog/batch", `{"items": nope`, "ns"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandlerBatchIngest_NamespaceNotFound_404(t *testing.T) {
	h := &Handler{service: &fakeIngester{batchErr: ErrNamespaceNotFound}}
	rec := httptest.NewRecorder()
	h.BatchIngest(rec, newCatalogPathRequest(http.MethodPost, "/catalog/batch",
		`{"items":[{"object_id":"o1","content":"hi"}]}`, "ns"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandlerListObjects_ParsesParams(t *testing.T) {
	fake := &fakeIngester{listResp: &codohuetypes.CatalogObjectsResponse{
		Namespace: "ns", Total: 1, Limit: 50, Offset: 0,
		Items: []codohuetypes.CatalogObjectSummary{{ObjectID: "o1", UpdatedAt: "2026-01-02T03:04:05Z"}},
	}}
	h := &Handler{service: fake}
	rec := httptest.NewRecorder()
	req := newCatalogPathRequest(http.MethodGet, "/catalog/objects?changed_since=2026-01-01T00:00:00Z&limit=50", "", "ns")
	h.ListObjects(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.lastSince == nil || !fake.lastSince.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("changed_since not parsed: %v", fake.lastSince)
	}
	if fake.lastLimit != 50 {
		t.Fatalf("limit not passed: %d", fake.lastLimit)
	}
	if !strings.Contains(rec.Body.String(), `"o1"`) {
		t.Errorf("items missing from body: %s", rec.Body.String())
	}
}

func TestHandlerListObjects_BadChangedSince_400(t *testing.T) {
	h := &Handler{service: &fakeIngester{}}
	rec := httptest.NewRecorder()
	h.ListObjects(rec, newCatalogPathRequest(http.MethodGet, "/catalog/objects?changed_since=yesterday", "", "ns"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandlerListObjects_BadLimitAndOffset_400(t *testing.T) {
	h := &Handler{service: &fakeIngester{}}
	for _, q := range []string{"?limit=zero", "?limit=-3", "?offset=x", "?offset=-1"} {
		rec := httptest.NewRecorder()
		h.ListObjects(rec, newCatalogPathRequest(http.MethodGet, "/catalog/objects"+q, "", "ns"))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("query %q: expected 400, got %d", q, rec.Code)
		}
	}
}

func TestHandlerListObjects_MissingNamespace_400(t *testing.T) {
	h := &Handler{service: &fakeIngester{}}
	rec := httptest.NewRecorder()
	h.ListObjects(rec, newCatalogPathRequest(http.MethodGet, "/catalog/objects", "", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandlerListObjects_NamespaceNotFound_404(t *testing.T) {
	h := &Handler{service: &fakeIngester{listErr: ErrNamespaceNotFound}}
	rec := httptest.NewRecorder()
	h.ListObjects(rec, newCatalogPathRequest(http.MethodGet, "/catalog/objects", "", "ghost"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestHandlerListObjects_LimitCappedAt1000(t *testing.T) {
	fake := &fakeIngester{listResp: &codohuetypes.CatalogObjectsResponse{Namespace: "ns"}}
	h := &Handler{service: fake}
	rec := httptest.NewRecorder()
	h.ListObjects(rec, newCatalogPathRequest(http.MethodGet, "/catalog/objects?limit=99999", "", "ns"))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if fake.lastLimit != 1000 {
		t.Fatalf("limit must cap at 1000, got %d", fake.lastLimit)
	}
}

func TestHandlerBatchIngest_MissingNamespace_400(t *testing.T) {
	h := &Handler{service: &fakeIngester{}}
	rec := httptest.NewRecorder()
	h.BatchIngest(rec, newCatalogPathRequest(http.MethodPost, "/catalog/batch", `{"items":[]}`, ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
