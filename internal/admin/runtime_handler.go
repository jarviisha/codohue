package admin

import (
	"context"
	"github.com/jarviisha/codohue/internal/config"
	"github.com/jarviisha/codohue/internal/core/httpapi"
	"net/http"
	"time"
)

// SetRuntimeReader connects the optional runtime reporting store during startup.
func (h *Handler) SetRuntimeReader(reader func(context.Context) ([]config.RuntimeSnapshot, error)) {
	h.runtimeReader = reader
}

// GetRuntime returns recent process self-reports; no deployment values are inferred.
func (h *Handler) GetRuntime(w http.ResponseWriter, r *http.Request) {
	if h.runtimeReader == nil {
		httpapi.WriteError(w, 503, "runtime_unavailable", "runtime reports unavailable")
		return
	}
	snapshots, err := h.runtimeReader(r.Context())
	if err != nil {
		httpapi.WriteError(w, 503, "runtime_unavailable", "runtime reports unavailable")
		return
	}
	if snapshots == nil {
		snapshots = []config.RuntimeSnapshot{}
	}
	httpapi.WriteJSON(w, 200, struct {
		Processes     []config.RuntimeSnapshot `json:"processes"`
		ObservedAt    time.Time                `json:"observed_at"`
		ExpirySeconds int                      `json:"expiry_seconds"`
	}{snapshots, time.Now().UTC(), int(config.RuntimeReportTTL.Seconds())})
}
