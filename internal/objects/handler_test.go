package objects

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

type fakeUpserter struct {
	obj *Object
	err error
}

func (f *fakeUpserter) Upsert(_ context.Context, _, _ string, _ *UpsertRequest) (*Object, error) {
	return f.obj, f.err
}

func newRequest(t *testing.T, ns, id, body string) *http.Request {
	t.Helper()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPut,
		"/v1/namespaces/"+ns+"/objects/"+id, bytes.NewBufferString(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("ns", ns)
	rctx.URLParams.Add("id", id)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestUpsertHandler_OK(t *testing.T) {
	h := NewHandler(&fakeUpserter{obj: &Object{
		Namespace: "ns", ObjectID: "o1", AuthorSubjectID: "u1",
		UpdatedAt: time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC),
	}})
	rec := httptest.NewRecorder()

	h.Upsert(rec, newRequest(t, "ns", "o1", `{"author_subject_id":"u1"}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
	}
	var resp codohuetypes.ObjectResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.AuthorSubjectID != "u1" || resp.ObjectID != "o1" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

// Unknown fields must fail loudly — the wire contract is strict-decoded so a
// client typo does not get silently dropped.
func TestUpsertHandler_RejectsUnknownField(t *testing.T) {
	h := NewHandler(&fakeUpserter{obj: &Object{}})
	rec := httptest.NewRecorder()

	h.Upsert(rec, newRequest(t, "ns", "o1", `{"author_subject_id":"u1","namespace":"ns"}`))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for an unknown field, got %d", rec.Code)
	}
}

func TestUpsertHandler_RejectsInvalidJSON(t *testing.T) {
	h := NewHandler(&fakeUpserter{obj: &Object{}})
	rec := httptest.NewRecorder()

	h.Upsert(rec, newRequest(t, "ns", "o1", `{`))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestUpsertHandler_MapsInvalidRequestTo400(t *testing.T) {
	h := NewHandler(&fakeUpserter{err: ErrInvalidRequest})
	rec := httptest.NewRecorder()

	h.Upsert(rec, newRequest(t, "ns", "o1", `{"author_subject_id":"u1"}`))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestUpsertHandler_MapsUnknownErrorTo500(t *testing.T) {
	h := NewHandler(&fakeUpserter{err: errors.New("db down")})
	rec := httptest.NewRecorder()

	h.Upsert(rec, newRequest(t, "ns", "o1", `{"author_subject_id":"u1"}`))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
