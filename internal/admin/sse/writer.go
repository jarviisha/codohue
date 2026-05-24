package sse

import (
	"context"
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
		return err
	}
	sw.f.Flush()
	return nil
}

// RunHeartbeat blocks sending Ping every interval until ctx is cancelled or
// the client disconnects. Spawn it from the handler goroutine after primary
// event sources are wired:
//
//	go sse.RunHeartbeat(ctx, writer, 15*time.Second)
func RunHeartbeat(ctx context.Context, w *Writer, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.Done():
			return
		case <-t.C:
			if err := w.Ping(); err != nil {
				return
			}
		}
	}
}
