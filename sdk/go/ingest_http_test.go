package codohue

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

func TestNamespaceIngestEvent(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		if r.URL.Path != "/v1/namespaces/feed/events" {
			t.Errorf("path = %s", r.URL.Path)
		}
		var body codohuetypes.EventPayload
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.Namespace != "feed" {
			t.Errorf("namespace not overridden: %q", body.Namespace)
		}
		if body.Action != codohuetypes.ActionLike {
			t.Errorf("action = %q", body.Action)
		}
		w.WriteHeader(http.StatusAccepted)
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, _ := New(srv.URL)
	err := c.Namespace("feed", "k").IngestEvent(context.Background(), codohuetypes.EventPayload{
		// Namespace intentionally left empty — SDK must populate it.
		SubjectID: "u",
		ObjectID:  "o",
		Action:    codohuetypes.ActionLike,
		Timestamp: time.Now(),
	})
	if err != nil {
		t.Fatalf("IngestEvent: %v", err)
	}
}

func TestNamespaceIngestEventOverridesMismatchedNamespace(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body codohuetypes.EventPayload
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Namespace != "feed" {
			t.Errorf("expected overridden namespace 'feed', got %q", body.Namespace)
		}
		w.WriteHeader(http.StatusAccepted)
	})
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	c, _ := New(srv.URL)
	err := c.Namespace("feed", "k").IngestEvent(context.Background(), codohuetypes.EventPayload{
		Namespace: "wrong", // must be overwritten
		SubjectID: "u",
		ObjectID:  "o",
		Action:    codohuetypes.ActionView,
	})
	if err != nil {
		t.Fatalf("IngestEvent: %v", err)
	}
}
