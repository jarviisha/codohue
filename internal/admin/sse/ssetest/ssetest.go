package ssetest

import (
	"bufio"
	"io"
	"strings"
	"testing"
	"time"
)

// Event is one parsed SSE message.
type Event struct {
	ID   string
	Name string
	Data string
}

// Read consumes the next n events from body. Fails the test if the stream
// closes before n events arrive or no progress is made within timeout.
//
// Typical usage in an integration test:
//
//	resp := httptest.NewServer(handler).Client().Get(url)
//	events := ssetest.Read(t, resp.Body, 3, 2*time.Second)
//	if events[0].Name != "phase_started" { ... }
func Read(t *testing.T, body io.Reader, n int, timeout time.Duration) []Event {
	t.Helper()
	out := make([]Event, 0, n)
	done := make(chan struct{})
	var readErr error

	go func() {
		defer close(done)
		sc := bufio.NewScanner(body)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		var cur Event
		for sc.Scan() {
			line := sc.Text()
			if line == "" {
				if cur != (Event{}) {
					out = append(out, cur)
					cur = Event{}
					if len(out) >= n {
						return
					}
				}
				continue
			}
			switch {
			case strings.HasPrefix(line, "id: "):
				cur.ID = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "event: "):
				cur.Name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				cur.Data = strings.TrimPrefix(line, "data: ")
			}
		}
		readErr = sc.Err()
	}()

	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("ssetest: timed out after %v waiting for %d events (got %d)", timeout, n, len(out))
	}
	if readErr != nil {
		t.Fatalf("ssetest: scan error: %v", readErr)
	}
	if len(out) < n {
		t.Fatalf("ssetest: stream closed with %d events, want %d", len(out), n)
	}
	return out
}
