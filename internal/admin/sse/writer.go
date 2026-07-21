package sse

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// ErrNotFlushable is returned when the http.ResponseWriter does not implement
// http.Flusher. Typical of HTTP/2 with intermediate buffering middleware.
var ErrNotFlushable = errors.New("sse: response writer is not a flusher")

// Writer streams Server-Sent Events to a single HTTP client.
type Writer struct {
	w  http.ResponseWriter
	f  http.Flusher
	r  *http.Request
	mu sync.Mutex
}

// NewWriter sets SSE headers, flushes them, and returns a Writer ready for
// Send/Ping calls. Returns ErrNotFlushable when the response writer cannot flush.
//
// The per-handler write deadline is cleared via http.ResponseController — the
// server's WriteTimeout would otherwise kill a long-lived SSE connection at
// the deadline (the deadline is fixed from request start and does NOT reset
// on each write). Servers that wrap the response writer in a way that
// doesn't surface SetWriteDeadline still get a working stream; we just can't
// extend the deadline and the connection dies at WriteTimeout.
func NewWriter(w http.ResponseWriter, r *http.Request) (*Writer, error) {
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, ErrNotFlushable
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")

	// Opt out of the server's WriteTimeout for this connection — SSE streams
	// stay open for the lifetime of the operator's session, which is well
	// beyond any reasonable read/write deadline. Errors are silently ignored:
	// not all response writers support deadlines (e.g. httptest recorders),
	// and the stream still functions, it just inherits whatever deadline the
	// server set.
	rc := http.NewResponseController(w)
	//nolint:errcheck // deadline opt-out is best-effort; recorders without deadline support just keep the server default and the stream still works (see comment above)
	rc.SetWriteDeadline(time.Time{})

	w.WriteHeader(http.StatusOK)
	f.Flush()
	return &Writer{w: w, f: f, r: r}, nil
}

// Done returns a channel closed when the client disconnects (the request
// context is cancelled).
func (sw *Writer) Done() <-chan struct{} {
	return sw.r.Context().Done()
}

// Send marshals data to JSON and writes one SSE event. Concurrent Send calls
// are serialized.
func (sw *Writer) Send(event string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("sse: marshal payload: %w", err)
	}
	return sw.sendRaw(event, "", payload)
}

// SendWithID is Send with a client-side reconnect ID. Browsers replay the last
// seen ID via the Last-Event-ID header on reconnect.
func (sw *Writer) SendWithID(event, id string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("sse: marshal payload: %w", err)
	}
	return sw.sendRaw(event, id, payload)
}

// Ping writes a heartbeat event with empty payload.
func (sw *Writer) Ping() error {
	return sw.sendRaw("ping", "", []byte("{}"))
}

func (sw *Writer) sendRaw(event, id string, data []byte) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	var buf []byte
	if id != "" {
		buf = append(buf, "id: "...)
		buf = append(buf, id...)
		buf = append(buf, '\n')
	}
	if event != "" {
		buf = append(buf, "event: "...)
		buf = append(buf, event...)
		buf = append(buf, '\n')
	}
	buf = append(buf, "data: "...)
	buf = append(buf, data...)
	buf = append(buf, '\n', '\n')
	if _, err := sw.w.Write(buf); err != nil {
		return fmt.Errorf("sse: write event: %w", err)
	}
	sw.f.Flush()
	return nil
}
