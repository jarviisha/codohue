package admin

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jarviisha/codohue/internal/admin/eventbus"
	"github.com/jarviisha/codohue/internal/admin/sse"
	"github.com/jarviisha/codohue/internal/core/httpapi"
	"github.com/jarviisha/codohue/internal/infra/metrics"
)

// sseHeartbeatInterval is how often each SSE stream emits a `ping` event
// while idle. Browsers + proxies use this to detect dead connections; the
// useServerStream client hook treats >45s without a ping as a force-reconnect
// signal.
const sseHeartbeatInterval = 15 * time.Second

// StreamBatchRun handles GET /api/admin/v1/batch-runs/{id}/stream.
//
//   - 204 No Content when the run is already terminal — the SPA falls back to
//     the snapshot endpoint (BUILD_PLAN §3.2).
//   - 404 when the run id does not exist.
//   - 503 when the event bus is not wired (defensive; main.go always wires it).
//   - 200 text/event-stream otherwise; closes when the run reaches a terminal
//     state or the client disconnects.
//
// Events forwarded to the client: phase_started, phase_completed, log_line,
// run_completed, cancelled. The handler stops streaming on the run_completed
// / cancelled event after flushing it through.
func (h *Handler) StreamBatchRun(w http.ResponseWriter, r *http.Request) {
	if h.bus == nil {
		httpapi.WriteError(w, http.StatusServiceUnavailable, "stream_unavailable", "event bus is not wired")
		return
	}
	id, ok := parseRunID(w, r)
	if !ok {
		return
	}

	// Subscribe BEFORE reading the snapshot: the other order loses the
	// terminal event when the run completes between the two, leaving the
	// client on a stream that only ever emits pings.
	entityID := strconv.FormatInt(id, 10)
	events, cancel := h.bus.Subscribe(eventbus.Filter{
		EntityID: entityID,
		Kinds: []string{
			"batch_run.phase_started",
			"batch_run.phase_completed",
			"batch_run.log_line",
			"batch_run.completed",
			"batch_run.cancelled",
		},
	})
	defer cancel()

	run, err := h.svc.GetBatchRunDetail(r.Context(), id)
	if err != nil {
		writeInternalError(w, r, "could not load batch run", err, slog.Int64("batch_run_id", id))
		return
	}
	if run == nil {
		httpapi.WriteError(w, http.StatusNotFound, "not_found", "batch run not found")
		return
	}
	if run.CompletedAt != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	streamRun(w, r, events, "batch_run", true)
}

// StreamOps handles GET /api/admin/v1/stream — the global ops bus that drives
// sidebar badges, toast notifications, and recent-items in the SPA. Filters
// at the bus level to the run-lifecycle + catalog-alert kinds; other
// internal events stay private to handlers that care.
func (h *Handler) StreamOps(w http.ResponseWriter, r *http.Request) {
	if h.bus == nil {
		httpapi.WriteError(w, http.StatusServiceUnavailable, "stream_unavailable", "event bus is not wired")
		return
	}
	events, cancel := h.bus.Subscribe(eventbus.Filter{
		Kinds: []string{
			"batch_run.started",
			"batch_run.completed",
			"batch_run.cancelled",
			"catalog.dead_letter_grew",
			"catalog.reembed_progress",
		},
	})
	defer cancel()
	// closeOnTerminal=false: this is the always-on fleet bus. Closing it on
	// any run's completion would tear the sidebar's connection down on every
	// finished run and drop events during the client's reconnect.
	streamRun(w, r, events, "ops", false)
}

// StreamCatalog handles GET /api/admin/v1/namespaces/{ns}/catalog/stream —
// pushes catalog item state changes for one namespace. Unlike the batch run
// stream, the catalog stream has no terminal kind — it stays alive until
// the client disconnects.
//
//   - 400 when ns is missing.
//   - 503 when the event bus is not wired (defensive; main.go always wires it).
//   - 200 text/event-stream otherwise.
//
// Events forwarded: item_state_changed (per-item transition), plus
// dead_letter_grew + backlog_snapshot when those signals come online.
func (h *Handler) StreamCatalog(w http.ResponseWriter, r *http.Request) {
	if h.bus == nil {
		httpapi.WriteError(w, http.StatusServiceUnavailable, "stream_unavailable", "event bus is not wired")
		return
	}
	ns := chi.URLParam(r, "ns")
	if ns == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_request", "namespace is required")
		return
	}
	events, cancel := h.bus.Subscribe(eventbus.Filter{
		Namespace: ns,
		Kinds: []string{
			"catalog.item_state_changed",
			"catalog.dead_letter_grew",
			"catalog.backlog_snapshot",
			"catalog.reembed_progress",
		},
	})
	defer cancel()
	streamRun(w, r, events, "catalog", false)
}

// streamRun is the shared writer loop used by every stream endpoint. It opens
// an SSE connection and ships every event from the bus channel until the
// channel closes (cancelled subscription) or the client disconnects.
//
// closeOnTerminal ends the stream when a run reaches a final state. Only the
// per-run stream wants that: the fleet-wide ops bus and the catalog stream
// are long-lived and must survive any single run completing.
//
// The `stream` label is fed straight into the Prometheus collectors so the
// active-connection gauge + dropped counter can be sliced by stream kind.
func streamRun(w http.ResponseWriter, r *http.Request, events <-chan eventbus.Event, stream string, closeOnTerminal bool) {
	sw, err := sse.NewWriter(w, r)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "sse_unsupported", err.Error())
		return
	}

	metrics.AdminSSEConnectionsActive.WithLabelValues(stream).Inc()
	defer metrics.AdminSSEConnectionsActive.WithLabelValues(stream).Dec()

	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if err := sw.Ping(); err != nil {
				metrics.AdminSSEDroppedTotal.WithLabelValues(stream, "client_slow").Inc()
				return
			}
		case e, ok := <-events:
			if !ok {
				return
			}
			name := sseEventName(e.Kind)
			if err := sw.Send(name, e.Payload); err != nil {
				metrics.AdminSSEDroppedTotal.WithLabelValues(stream, "client_slow").Inc()
				return
			}
			if closeOnTerminal && isTerminalKind(e.Kind) {
				return
			}
		}
	}
}

// sseEventName strips the bus's kind namespace prefix ("batch_run." or
// "catalog.") so the client-side switch can key off "phase_started" or
// "item_state_changed" rather than the fully-qualified bus kind. Other event
// kinds pass through unchanged.
func sseEventName(kind string) string {
	for _, prefix := range []string{"batch_run.", "catalog."} {
		if strings.HasPrefix(kind, prefix) {
			return kind[len(prefix):]
		}
	}
	return kind
}

// isTerminalKind reports whether observing this kind on a per-run subscription
// means we can close the stream — the run has reached a final state.
func isTerminalKind(kind string) bool {
	return kind == "batch_run.completed" || kind == "batch_run.cancelled"
}

// chi import kept for parity with handler.go pattern even when only used
// indirectly via parseRunID (helps grep find the handler file).
var _ = chi.URLParam
