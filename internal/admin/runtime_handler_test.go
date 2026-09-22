package admin

import (
	"context"
	"errors"
	"github.com/jarviisha/codohue/internal/config"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRuntimeHandlerUnavailableAndSnapshots(t *testing.T) {
	h := NewHandler(nil, "", nil)
	for _, tc := range []struct {
		name     string
		reader   func(context.Context) ([]config.RuntimeSnapshot, error)
		status   int
		contains string
	}{
		{name: "not configured", status: 503, contains: "runtime_unavailable"},
		{name: "store failed", reader: func(context.Context) ([]config.RuntimeSnapshot, error) {
			return nil, errors.New("secret connection details")
		}, status: 503, contains: "runtime_unavailable"},
		{name: "no reports", reader: func(context.Context) ([]config.RuntimeSnapshot, error) { return nil, nil }, status: 200, contains: `"processes":[]`},
		{name: "reported", reader: func(context.Context) ([]config.RuntimeSnapshot, error) {
			return []config.RuntimeSnapshot{{Process: "cron", Settings: []config.RuntimeSetting{{Name: "batch_interval_minutes", Value: 5}}}}, nil
		}, status: 200, contains: `"batch_interval_minutes"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h.SetRuntimeReader(tc.reader)
			w := httptest.NewRecorder()
			h.GetRuntime(w, httptest.NewRequest("GET", "/api/admin/v1/runtime", nil))
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.contains) || strings.Contains(w.Body.String(), "secret connection") {
				t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
			}
		})
	}
}
