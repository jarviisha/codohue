package catalog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

type fakeIngester struct {
	item *CatalogItem
	err  error

	lastNS  string
	lastReq *IngestRequest
}

func (f *fakeIngester) Ingest(_ context.Context, ns string, req *IngestRequest) (*CatalogItem, error) {
	f.lastNS = ns
	f.lastReq = req
	return f.item, f.err
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
	h := &Handler{service: &fakeIngester{item: &CatalogItem{ID: 1, Namespace: "ns", ObjectID: "o1"}}}
	rec := httptest.NewRecorder()
	h.Ingest(rec, newCatalogRequest(`{"object_id":"o1","content":"hello"}`, "ns"))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Errorf("expected empty body on 202, got %q", rec.Body.String())
	}
}

func TestHandlerIngest_PathNamespacePrevailsOverBody(t *testing.T) {
	// If the body sneaks a "namespace" field, the URL path is the only
	// source of truth — consistent with the 003 RESTful redesign.
	ing := &fakeIngester{item: &CatalogItem{ID: 1}}
	h := &Handler{service: ing}
	rec := httptest.NewRecorder()
	h.Ingest(rec, newCatalogRequest(`{"object_id":"o1","content":"hi","namespace":"WRONG"}`, "real-ns"))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
	if ing.lastNS != "real-ns" {
		t.Errorf("expected service to receive ns from path, got %q", ing.lastNS)
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
