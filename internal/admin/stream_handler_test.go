package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jarviisha/codohue/internal/admin/eventbus"
	"github.com/jarviisha/codohue/internal/admin/sse/ssetest"
)

func TestStreamBatchRunReturns503WhenBusNotWired(t *testing.T) {
	h := newTestHandler(&fakeSvc{
		batchRunDetail: &BatchRunDetail{
			BatchRunSummary: BatchRunSummary{ID: 1},
		},
	})
	// bus intentionally unset

	req := newChiRequest(http.MethodGet, "/api/admin/v1/batch-runs/1/stream",
		map[string]string{"id": "1"}, "")
	rec := httptest.NewRecorder()
	h.StreamBatchRun(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503", rec.Code)
	}
}

func TestStreamBatchRunReturns204WhenTerminal(t *testing.T) {
	now := time.Now()
	svc := &fakeSvc{
		batchRunDetail: &BatchRunDetail{
			BatchRunSummary: BatchRunSummary{ID: 1, CompletedAt: &now},
		},
	}
	h := newTestHandler(svc)
	bus := eventbus.NewBus()
	defer bus.Close()
	h.SetEventBus(bus)

	req := newChiRequest(http.MethodGet, "/api/admin/v1/batch-runs/1/stream",
		map[string]string{"id": "1"}, "")
	rec := httptest.NewRecorder()
	h.StreamBatchRun(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status=%d, want 204", rec.Code)
	}
}

func TestStreamBatchRunReturns404WhenRunMissing(t *testing.T) {
	h := newTestHandler(&fakeSvc{batchRunDetail: nil})
	bus := eventbus.NewBus()
	defer bus.Close()
	h.SetEventBus(bus)

	req := newChiRequest(http.MethodGet, "/api/admin/v1/batch-runs/99/stream",
		map[string]string{"id": "99"}, "")
	rec := httptest.NewRecorder()
	h.StreamBatchRun(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", rec.Code)
	}
}

func TestStreamBatchRunEmitsEventsAndClosesOnTerminal(t *testing.T) {
	svc := &fakeSvc{
		batchRunDetail: &BatchRunDetail{
			BatchRunSummary: BatchRunSummary{ID: 1, Namespace: "prod"},
		},
	}
	h := newTestHandler(svc)
	bus := eventbus.NewBus()
	defer bus.Close()
	h.SetEventBus(bus)

	mux := http.NewServeMux()
	mux.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		// Inject chi URL params into the real request's context — the
		// httptest server bypasses chi router so we wire the param manually.
		// Crucially we keep the original request context (so the listener's
		// cancel propagates) and only ADD the chi route context value.
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", "1")
		r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
		h.StreamBatchRun(w, r)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/stream")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	// Give the handler a beat to subscribe before publishing.
	time.Sleep(50 * time.Millisecond)
	bus.Publish(context.Background(), eventbus.Event{
		Kind:     "batch_run.phase_started",
		EntityID: "1",
		Payload:  map[string]any{"phase": 1},
	})
	bus.Publish(context.Background(), eventbus.Event{
		Kind:     "batch_run.completed",
		EntityID: "1",
		Payload:  map[string]any{"success": true},
	})

	events := ssetest.Read(t, resp.Body, 2, 3*time.Second)
	if events[0].Name != "phase_started" {
		t.Errorf("events[0].Name=%q, want phase_started", events[0].Name)
	}
	if events[1].Name != "completed" {
		t.Errorf("events[1].Name=%q, want completed", events[1].Name)
	}
}

func TestStreamOpsReturns503WhenBusNotWired(t *testing.T) {
	h := newTestHandler(&fakeSvc{})
	req := newChiRequest(http.MethodGet, "/api/admin/v1/stream", nil, "")
	rec := httptest.NewRecorder()
	h.StreamOps(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d, want 503", rec.Code)
	}
}

func TestStreamOpsForwardsRunLifecycleEvents(t *testing.T) {
	h := newTestHandler(&fakeSvc{})
	bus := eventbus.NewBus()
	defer bus.Close()
	h.SetEventBus(bus)

	srv := httptest.NewServer(http.HandlerFunc(h.StreamOps))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	time.Sleep(50 * time.Millisecond)
	bus.Publish(context.Background(), eventbus.Event{
		Kind:    "batch_run.started",
		Payload: map[string]any{"id": 42, "namespace": "prod"},
	})
	// StreamOps closes on terminal kinds (completed/cancelled) — emit
	// completed so the goroutine exits and ssetest.Read finishes cleanly.
	bus.Publish(context.Background(), eventbus.Event{
		Kind:    "batch_run.completed",
		Payload: map[string]any{"success": true},
	})

	events := ssetest.Read(t, resp.Body, 2, 3*time.Second)
	if events[0].Name != "started" {
		t.Errorf("events[0].Name=%q, want started", events[0].Name)
	}
}
