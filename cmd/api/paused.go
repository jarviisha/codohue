package main

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/jarviisha/codohue/internal/core/httpapi"
	"github.com/jarviisha/codohue/internal/core/namespace"
)

// rejectPausedNamespace answers 409 namespace_paused for every data-plane
// route of a namespace an operator has paused. It runs after authentication
// so an unauthenticated caller cannot probe the pause state.
// ponytail: one namespace_configs read per request; cache it if that read shows up in latency.
func rejectPausedNamespace(getConfig func(context.Context, string) (*namespace.Config, error)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ns := httpapi.URLParam(r, "ns")
			if ns == "" {
				next.ServeHTTP(w, r)
				return
			}
			cfg, err := getConfig(r.Context(), ns)
			if err != nil {
				slog.Error("paused check failed", "namespace", ns, "error", err)
				httpapi.WriteError(w, http.StatusServiceUnavailable, "namespace_config_unavailable", "namespace configuration is unavailable")
				return
			}
			if cfg != nil && cfg.Paused {
				httpapi.WriteNamespacePaused(w)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
