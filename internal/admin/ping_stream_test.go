package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jarviisha/codohue/internal/admin/sse/ssetest"
)

func TestPingStreamEmitsTickEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(PingStream))
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck // test cleanup; close error is irrelevant to assertions

	if got := resp.Header.Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("Content-Type=%q, want text/event-stream", got)
	}

	events := ssetest.Read(t, resp.Body, 2, 3*time.Second)
	for i, e := range events {
		if e.Name != "tick" {
			t.Errorf("event[%d].Name=%q, want tick", i, e.Name)
		}
		if !strings.Contains(e.Data, `"at":`) {
			t.Errorf("event[%d].Data=%q missing at field", i, e.Data)
		}
	}
}
