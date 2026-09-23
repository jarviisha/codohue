package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jarviisha/codohue/internal/config"
	"github.com/jarviisha/codohue/internal/core/httpapi"
	"github.com/jarviisha/codohue/internal/core/namespace"
)

// ConfigurationStore is implemented by the namespace adapter in cmd/admin.
type ConfigurationStore interface {
	ReadConfiguration(context.Context, string) (*namespace.Configuration, error)
	PatchConfiguration(context.Context, string, *namespace.ConfigurationPatch, bool) (*namespace.Configuration, error)
}

// SetConfigurationStore wires the independently versioned configuration API.
func (h *Handler) SetConfigurationStore(store ConfigurationStore) { h.configuration = store }

// GetConfiguration reads all configuration groups without exposing credentials.
func (h *Handler) GetConfiguration(w http.ResponseWriter, r *http.Request) {
	if h.configuration == nil {
		httpapi.WriteError(w, 503, "unavailable", "Configuration is unavailable")
		return
	}
	out, err := h.configuration.ReadConfiguration(r.Context(), chi.URLParam(r, "ns"))
	h.writeConfiguration(w, r, out, err)
}

// PatchConfiguration persists one group under its revision precondition.
func (h *Handler) PatchConfiguration(w http.ResponseWriter, r *http.Request) {
	h.changeConfiguration(w, r, false)
}

// ValidateConfiguration checks the candidate with the same rules as persistence.
func (h *Handler) ValidateConfiguration(w http.ResponseWriter, r *http.Request) {
	h.changeConfiguration(w, r, true)
}
func (h *Handler) changeConfiguration(w http.ResponseWriter, r *http.Request, validation bool) {
	if h.configuration == nil {
		httpapi.WriteError(w, 503, "unavailable", "Configuration is unavailable")
		return
	}
	var req *namespace.ConfigurationPatch
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil || req == nil {
		httpapi.WriteError(w, 422, "invalid_configuration", "Invalid configuration request")
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		httpapi.WriteError(w, 422, "invalid_configuration", "Expected one JSON object")
		return
	}
	out, err := h.configuration.PatchConfiguration(r.Context(), chi.URLParam(r, "ns"), req, validation)
	h.writeConfiguration(w, r, out, err)
}
func (h *Handler) writeConfiguration(w http.ResponseWriter, r *http.Request, out *namespace.Configuration, err error) {
	if err != nil {
		var configErr *namespace.ConfigurationError
		if errors.As(err, &configErr) {
			// A conflict snapshot is installed as the client's canonical cache,
			// so it needs the same observations a successful read carries.
			h.configurationDefaults(r.Context(), configErr.Current)
			httpapi.WriteJSON(w, configErr.Status, map[string]any{"error": configErr})
			return
		}
		writeInternalError(w, r, "Configuration request failed", err)
		return
	}
	h.configurationDefaults(r.Context(), out)
	httpapi.WriteJSON(w, http.StatusOK, out)
}

func (h *Handler) configurationDefaults(ctx context.Context, out *namespace.Configuration) {
	if out == nil {
		return
	}
	out.Defaults = map[string]namespace.ConfigurationDefault{}
	var reports []config.RuntimeSnapshot
	if h.runtimeReader != nil {
		//nolint:errcheck // a failed read is reported as an unknown observation below
		reports, _ = h.runtimeReader(ctx)
	}
	for field, source := range map[string][2]string{"catalog_max_attempts": {"embedder", "max_attempts_default"}, "catalog_max_content_bytes": {"api", "catalog_max_content_bytes"}} {
		observation := namespace.ConfigurationDefault{State: "unknown", Process: source[0], Reports: []namespace.ConfigurationDefaultReport{}}
		missing := false
		for _, report := range reports {
			if report.Process != source[0] || time.Since(report.ReportedAt) > config.RuntimeReportTTL {
				continue
			}
			found := false
			for _, setting := range report.Settings {
				if setting.Name == source[1] {
					found = true
					observation.Reports = append(observation.Reports, namespace.ConfigurationDefaultReport{Instance: report.Instance, ReportedAt: report.ReportedAt.Format(time.RFC3339), Value: setting.Value})
				}
			}
			if !found {
				missing = true
			}
		}
		if len(observation.Reports) > 0 && !missing {
			observation.State = "observed"
			observation.Value = observation.Reports[0].Value
			for _, r := range observation.Reports {
				if fmt.Sprint(r.Value) != fmt.Sprint(observation.Value) {
					observation.State = "mixed"
					observation.Value = nil
					break
				}
			}
		}
		out.Defaults[field] = observation
	}
}
