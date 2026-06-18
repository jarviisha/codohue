package admin

import (
	"context"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jarviisha/codohue/internal/admin/eventbus"
	"github.com/jarviisha/codohue/internal/admin/sse"
	"github.com/jarviisha/codohue/internal/core/httpapi"
	"github.com/jarviisha/codohue/internal/infra/metrics"
)

// eventsStreamLocalBuffer bounds the per-connection forward queue between the
// bus reader and the client writer. When the client can't keep up the queue
// fills and we drop + report via an `event: dropped` frame rather than letting
// a slow operator tab stall the whole bus subscription.
const eventsStreamLocalBuffer = 256

// sseSender is the subset of *sse.Writer the events stream loop needs; broken
// out so the loop is unit-testable without a real HTTP connection.
type sseSender interface {
	Send(event string, data any) error
	Ping() error
}

// StreamEvents handles GET /api/admin/v1/namespaces/{ns}/events/stream — the
// live ingest tail for one namespace. Optional ?action= and ?subject_id=
// filter server-side.
//
//   - 400 when ns is missing.
//   - 503 when the event bus is not wired.
//   - 200 text/event-stream otherwise; stays open until the client disconnects.
//
// Events forwarded: `event` (one per ingested event) and `dropped`
// ({"count": n}) when the client falls behind. Heartbeat `ping` every 15s.
func (h *Handler) StreamEvents(w http.ResponseWriter, r *http.Request) {
	if h.bus == nil {
		httpapi.WriteError(w, http.StatusServiceUnavailable, "stream_unavailable", "event bus is not wired")
		return
	}
	ns := chi.URLParam(r, "ns")
	if ns == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "namespace is required")
		return
	}
	q := r.URL.Query()
	actionFilter := q.Get("action")
	subjectFilter := q.Get("subject_id")

	events, cancel := h.bus.Subscribe(eventbus.Filter{
		Namespace: ns,
		Kinds:     []string{"events.ingested"},
	})
	defer cancel()

	sw, err := sse.NewWriter(w, r)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "sse_unsupported", err.Error())
		return
	}

	metrics.AdminSSEConnectionsActive.WithLabelValues("events").Inc()
	defer metrics.AdminSSEConnectionsActive.WithLabelValues("events").Dec()

	streamEvents(r.Context(), sw, events, actionFilter, subjectFilter)
}

// streamEvents is the testable core of the events tail. A reader goroutine
// pulls matching events off the bus into a bounded local queue (dropping +
// counting when full); the main loop writes the queue to the client and emits
// a `dropped` frame before the next `event` whenever drops accumulated.
func streamEvents(ctx context.Context, sw sseSender, events <-chan eventbus.Event, actionFilter, subjectFilter string) {
	forward := make(chan eventbus.Event, eventsStreamLocalBuffer)
	var dropped atomic.Int64

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-events:
				if !ok {
					return
				}
				if !matchEventFilter(e, actionFilter, subjectFilter) {
					continue
				}
				select {
				case forward <- e:
				default:
					dropped.Add(1)
					metrics.AdminSSEDroppedTotal.WithLabelValues("events", "client_slow").Inc()
				}
			}
		}
	}()

	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()

	flushDropped := func() bool {
		if n := dropped.Swap(0); n > 0 {
			if err := sw.Send("dropped", map[string]int64{"count": n}); err != nil {
				return false
			}
		}
		return true
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			if !flushDropped() {
				return
			}
			if err := sw.Ping(); err != nil {
				return
			}
		case e := <-forward:
			if !flushDropped() {
				return
			}
			if err := sw.Send("event", e.Payload); err != nil {
				metrics.AdminSSEDroppedTotal.WithLabelValues("events", "client_slow").Inc()
				return
			}
		}
	}
}

// matchEventFilter applies the optional action / subject_id filters against an
// events.ingested payload. A missing or non-map payload never matches.
func matchEventFilter(e eventbus.Event, actionFilter, subjectFilter string) bool {
	if actionFilter == "" && subjectFilter == "" {
		return true
	}
	m, ok := e.Payload.(map[string]any)
	if !ok {
		return false
	}
	if actionFilter != "" {
		if a, ok := m["action"].(string); !ok || a != actionFilter {
			return false
		}
	}
	if subjectFilter != "" {
		if s, ok := m["subject_id"].(string); !ok || s != subjectFilter {
			return false
		}
	}
	return true
}
