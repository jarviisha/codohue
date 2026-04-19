package nsconfig

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// fakeService implements nsConfigUpserter for testing.
type fakeService struct {
	resp *UpsertResponse
	err  error
}

func (f *fakeService) Upsert(_ context.Context, _ string, _ *UpsertRequest) (*UpsertResponse, error) {
	return f.resp, f.err
}

// newRequest builds a PUT request with a chi route context so that
// chi.URLParam(r, "namespace") resolves correctly.
func newRequest(body, namespace string) *http.Request {
	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPut,
		"/v1/config/namespaces/"+namespace,
		strings.NewReader(body),
	)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("namespace", namespace)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestHandlerUpsert_InvalidJSON(t *testing.T) {
	h := &Handler{service: &fakeService{}}
	rec := httptest.NewRecorder()

	h.Upsert(rec, newRequest("not-valid-json", "test-ns"))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestNewHandler(t *testing.T) {
	svc := &Service{}
	h := NewHandler(svc)
	if h == nil || h.service != svc {
		t.Fatal("expected handler to be initialized with service")
	}
}

func TestHandlerUpsert_Success(t *testing.T) {
	now := time.Now()
	h := &Handler{service: &fakeService{
		resp: &UpsertResponse{Namespace: "test-ns", UpdatedAt: now, APIKey: "plaintext-key"},
	}}
	rec := httptest.NewRecorder()

	body := `{"action_weights":{"VIEW":1},"lambda":0.05}`
	h.Upsert(rec, newRequest(body, "test-ns"))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var got UpsertResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Namespace != "test-ns" {
		t.Errorf("Namespace: got %q, want %q", got.Namespace, "test-ns")
	}
	if got.APIKey != "plaintext-key" {
		t.Errorf("APIKey: got %q, want %q", got.APIKey, "plaintext-key")
	}
}

func TestHandlerUpsert_Success_NoAPIKeyOnUpdate(t *testing.T) {
	h := &Handler{service: &fakeService{
		resp: &UpsertResponse{Namespace: "test-ns", APIKey: ""},
	}}
	rec := httptest.NewRecorder()

	h.Upsert(rec, newRequest(`{"lambda":0.05}`, "test-ns"))

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var got UpsertResponse
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.APIKey != "" {
		t.Errorf("expected empty APIKey on update, got %q", got.APIKey)
	}
}

func TestHandlerUpsert_ServiceError(t *testing.T) {
	h := &Handler{service: &fakeService{err: errors.New("db error")}}
	rec := httptest.NewRecorder()

	h.Upsert(rec, newRequest(`{"lambda":0.05}`, "test-ns"))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

func TestHandlerUpsert_ContentTypeJSON(t *testing.T) {
	h := &Handler{service: &fakeService{
		resp: &UpsertResponse{Namespace: "test-ns"},
	}}
	rec := httptest.NewRecorder()

	h.Upsert(rec, newRequest(`{}`, "test-ns"))

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type: got %q, want %q", ct, "application/json")
	}
}
