package admin

import (
	"net/http"
	"time"

	"github.com/jarviisha/codohue/internal/admin/sse"
	"github.com/jarviisha/codohue/internal/core/httpapi"
	"github.com/jarviisha/codohue/internal/infra/metrics"
)

// PingStream handles GET /api/admin/v1/ping/stream — emits one `tick` event
// per second carrying the server timestamp. Foundation smoke-test endpoint
// for the SSE pipeline (Phase 0). Not a production endpoint; mounted behind
// the same session middleware as everything else.
func PingStream(w http.ResponseWriter, r *http.Request) {
	sw, err := sse.NewWriter(w, r)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "sse_unsupported", err.Error())
		return
	}

	metrics.AdminSSEConnectionsActive.WithLabelValues("ping").Inc()
	defer metrics.AdminSSEConnectionsActive.WithLabelValues("ping").Dec()

	// Send an immediate first tick so clients get instant feedback that the
	// stream opened — useful for cross-origin / proxy debugging.
	if err := sw.Send("tick", map[string]string{"at": time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
		metrics.AdminSSEDroppedTotal.WithLabelValues("ping", "client_slow").Inc()
		return
	}

	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case now := <-t.C:
			if err := sw.Send("tick", map[string]string{"at": now.UTC().Format(time.RFC3339Nano)}); err != nil {
				metrics.AdminSSEDroppedTotal.WithLabelValues("ping", "client_slow").Inc()
				return
			}
		}
	}
}
