package admin

import (
	"context"
	"encoding/json"
	"github.com/jarviisha/codohue/internal/config"
	"github.com/jarviisha/codohue/internal/core/namespace"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type configurationFake struct {
	called     bool
	validation bool
	err        error
}

func (f *configurationFake) ReadConfiguration(context.Context, string) (*namespace.Configuration, error) {
	return &namespace.Configuration{Namespace: "test"}, f.err
}
func (f *configurationFake) PatchConfiguration(_ context.Context, _ string, _ *namespace.ConfigurationPatch, validation bool) (*namespace.Configuration, error) {
	f.called = true
	f.validation = validation
	return &namespace.Configuration{Namespace: "test"}, f.err
}
func TestConfigurationHandlerStrictParsing(t *testing.T) {
	for _, body := range []string{`{"unknown":true}`, `{} {}`, `[]`, `null`} {
		f := &configurationFake{}
		h := NewHandler(nil, "", nil)
		h.SetConfigurationStore(f)
		w := httptest.NewRecorder()
		h.PatchConfiguration(w, httptest.NewRequest("PATCH", "/", strings.NewReader(body)))
		if w.Code != 422 || f.called {
			t.Fatalf("body=%s status=%d called=%v", body, w.Code, f.called)
		}
	}
}
func TestConfigurationHandlerConflictAndValidation(t *testing.T) {
	f := &configurationFake{err: &namespace.ConfigurationError{Status: 409, Code: "configuration_conflict", Message: "Changed", Current: &namespace.Configuration{Generation: 2}}}
	h := NewHandler(nil, "", nil)
	h.SetConfigurationStore(f)
	w := httptest.NewRecorder()
	h.PatchConfiguration(w, httptest.NewRequest("PATCH", "/", strings.NewReader(`{"group":"trending","generation":1,"base_revision":1,"changes":{}}`)))
	var body struct {
		Error namespace.ConfigurationError `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if w.Code != 409 || body.Error.Current.Generation != 2 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	f.err = nil
	w = httptest.NewRecorder()
	h.ValidateConfiguration(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"group":"trending","generation":1,"base_revision":1,"changes":{}}`)))
	if w.Code != 200 || !f.validation {
		t.Fatal("validation not delegated")
	}
}

func TestConfigurationDefaultObservations(t *testing.T) {
	now := time.Now()
	h := NewHandler(nil, "", nil)
	reports := []config.RuntimeSnapshot{}
	h.SetRuntimeReader(func(context.Context) ([]config.RuntimeSnapshot, error) { return reports, nil })
	out := &namespace.Configuration{}
	h.configurationDefaults(context.Background(), out)
	if out.Defaults["catalog_max_attempts"].State != "unknown" {
		t.Fatal("invented default")
	}
	report := func(instance string, value int) config.RuntimeSnapshot {
		return config.RuntimeSnapshot{Process: "embedder", Instance: instance, ReportedAt: now, Settings: []config.RuntimeSetting{{Name: "max_attempts_default", Value: value}}}
	}
	reports = []config.RuntimeSnapshot{report("one", 5), report("two", 5)}
	h.configurationDefaults(context.Background(), out)
	if out.Defaults["catalog_max_attempts"].State != "observed" || out.Defaults["catalog_max_attempts"].Value != 5 {
		t.Fatal("consistent defaults not observed")
	}
	reports[1] = report("two", 9)
	h.configurationDefaults(context.Background(), out)
	if out.Defaults["catalog_max_attempts"].State != "mixed" || out.Defaults["catalog_max_attempts"].Value != nil {
		t.Fatal("mixed defaults guessed")
	}
	reports[1].ReportedAt = now.Add(-3 * time.Minute)
	h.configurationDefaults(context.Background(), out)
	if len(out.Defaults["catalog_max_attempts"].Reports) != 1 {
		t.Fatal("stale report included")
	}
}
