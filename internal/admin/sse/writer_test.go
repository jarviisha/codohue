package sse

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewWriterSetsHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	if _, err := NewWriter(rec, req); err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	h := rec.Header()
	for k, want := range map[string]string{
		"Content-Type":      "text/event-stream",
		"Cache-Control":     "no-cache",
		"Connection":        "keep-alive",
		"X-Accel-Buffering": "no",
	} {
		if got := h.Get(k); got != want {
			t.Errorf("header %s = %q, want %q", k, got, want)
		}
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status=%d, want 200", rec.Code)
	}
}

// nonFlushable wraps an http.ResponseWriter to deliberately hide Flusher.
type nonFlushable struct {
	http.ResponseWriter
}

func TestNewWriterRejectsNonFlushable(t *testing.T) {
	rec := httptest.NewRecorder()
	nf := &nonFlushable{ResponseWriter: rec}
	_, err := NewWriter(nf, httptest.NewRequest("GET", "/", nil))
	if err != ErrNotFlushable {
		t.Fatalf("err=%v, want ErrNotFlushable", err)
	}
}

func TestSendWritesSSEFormat(t *testing.T) {
	rec := httptest.NewRecorder()
	w, err := NewWriter(rec, httptest.NewRequest("GET", "/", nil))
	if err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if err := w.Send("test", map[string]int{"x": 1}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	want := "event: test\ndata: {\"x\":1}\n\n"
	if got := rec.Body.String(); !strings.Contains(got, want) {
		t.Fatalf("body=%q does not contain %q", got, want)
	}
}

func TestSendWithIDIncludesIDLine(t *testing.T) {
	rec := httptest.NewRecorder()
	w, _ := NewWriter(rec, httptest.NewRequest("GET", "/", nil))
	if err := w.SendWithID("test", "42", nil); err != nil {
		t.Fatal(err)
	}
	want := "id: 42\nevent: test\ndata: null\n\n"
	if got := rec.Body.String(); !strings.Contains(got, want) {
		t.Fatalf("body missing id+event+data: %q", got)
	}
}

func TestPingWritesPingEvent(t *testing.T) {
	rec := httptest.NewRecorder()
	w, _ := NewWriter(rec, httptest.NewRequest("GET", "/", nil))
	if err := w.Ping(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.Body.String(), "event: ping\ndata: {}\n\n") {
		t.Fatalf("ping not in body: %q", rec.Body.String())
	}
}

func TestDoneSignalsOnRequestContextCancel(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
	w, _ := NewWriter(rec, req)
	cancel()
	select {
	case <-w.Done():
	case <-time.After(time.Second):
		t.Fatal("Done() did not fire after context cancel")
	}
}

func TestRunHeartbeatPingsEveryInterval(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)
	w, _ := NewWriter(rec, req)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		RunHeartbeat(ctx, w, 30*time.Millisecond)
	}()
	time.Sleep(120 * time.Millisecond)
	cancel()
	wg.Wait()
	pings := strings.Count(rec.Body.String(), "event: ping")
	if pings < 2 {
		t.Fatalf("pings=%d, want >= 2; body=%q", pings, rec.Body.String())
	}
}

func TestRunHeartbeatExitsOnContextCancel(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/", nil).WithContext(context.Background())
	w, _ := NewWriter(rec, req)
	done := make(chan struct{})
	go func() {
		RunHeartbeat(ctx, w, time.Second)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("RunHeartbeat did not exit on ctx cancel")
	}
}

// deadlineRecorder wraps httptest.ResponseRecorder with a Flush + a
// SetWriteDeadline implementation so http.NewResponseController surfaces it.
// The recorder captures whatever deadline NewWriter sets.
type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadlineCalls []time.Time
}

func (d *deadlineRecorder) Flush() { d.ResponseRecorder.Flush() }

func (d *deadlineRecorder) SetWriteDeadline(t time.Time) error {
	d.deadlineCalls = append(d.deadlineCalls, t)
	return nil
}

func TestNewWriterClearsWriteDeadline(t *testing.T) {
	rec := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	req := httptest.NewRequest("GET", "/", nil)
	if _, err := NewWriter(rec, req); err != nil {
		t.Fatalf("NewWriter: %v", err)
	}
	if len(rec.deadlineCalls) != 1 {
		t.Fatalf("SetWriteDeadline called %d times, want 1", len(rec.deadlineCalls))
	}
	if !rec.deadlineCalls[0].IsZero() {
		t.Errorf("SetWriteDeadline called with %v, want zero (no deadline)", rec.deadlineCalls[0])
	}
}

func TestSendConcurrencyIsSerialized(t *testing.T) {
	rec := httptest.NewRecorder()
	w, _ := NewWriter(rec, httptest.NewRequest("GET", "/", nil))

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = w.Send("x", map[string]int{"n": n})
		}(i)
	}
	wg.Wait()
	// Race detector covers the actual safety; this just smokes that we got
	// twenty well-formed events out (each starts with "event: x" + newline-data + blank line).
	got := strings.Count(rec.Body.String(), "event: x\n")
	if got != 20 {
		t.Fatalf("got %d well-formed events, want 20; body=%q", got, rec.Body.String())
	}
}
