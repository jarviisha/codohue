package admin

import (
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

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close() //nolint:errcheck

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
