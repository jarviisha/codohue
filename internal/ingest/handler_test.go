package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jarviisha/codohue/internal/core/httpapi"
)

type fakeHTTPProcessor struct {
	err         error
	lastPayload *EventPayload
}

func (f *fakeHTTPProcessor) Process(_ context.Context, payload *EventPayload) error {
	f.lastPayload = payload
	return f.err
}

func newIngestRequest(body, namespace string) *http.Request {
	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/v1/namespaces/"+namespace+"/events",
		strings.NewReader(body),
	)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("ns", namespace)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestHandlerIngestSuccess(t *testing.T) {
	proc := &fakeHTTPProcessor{}
	h := &Handler{service: proc}
	rec := httptest.NewRecorder()

	h.Ingest(rec, newIngestRequest(`{"subject_id":"u1","object_id":"o1","action":"VIEW","timestamp":"2026-04-21T00:00:00Z"}`, "ns"))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
	if proc.lastPayload == nil || proc.lastPayload.Namespace != "ns" {
		t.Fatalf("payload namespace not injected: %+v", proc.lastPayload)
	}
}

func TestHandlerIngestNamespaceMismatch(t *testing.T) {
	h := &Handler{service: &fakeHTTPProcessor{}}
	rec := httptest.NewRecorder()

	h.Ingest(rec, newIngestRequest(`{"namespace":"other","subject_id":"u1","object_id":"o1","action":"VIEW","timestamp":"2026-04-21T00:00:00Z"}`, "ns"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	var got httpapi.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if got.Error.Code != "namespace_mismatch" {
		t.Fatalf("unexpected error code: %+v", got)
	}
}

func TestHandlerIngestClientError(t *testing.T) {
	h := &Handler{service: &fakeHTTPProcessor{err: fmt.Errorf("resolve weight: %w", ErrUnknownAction)}}
	rec := httptest.NewRecorder()

	h.Ingest(rec, newIngestRequest(`{"subject_id":"u1","object_id":"o1","action":"UNKNOWN","timestamp":"2026-04-21T00:00:00Z"}`, "ns"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestHandlerIngestInvalidJSON(t *testing.T) {
	h := &Handler{service: &fakeHTTPProcessor{}}
	rec := httptest.NewRecorder()

	h.Ingest(rec, newIngestRequest(`not-json`, "ns"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
