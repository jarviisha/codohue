package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jarviisha/codohue/internal/admin"
)

// TestSSE_ExitsOnBaseContextCancel pins the audited shutdown contract: the
// http.Server in cmd/admin wires its BaseContext to the app root ctx, so a
// cancel() on shutdown propagates straight into r.Context().Done() inside
// every SSE handler. Without BaseContext, srv.Shutdown would hang on the
// long-lived stream until the shutdown timeout.
//
// We exercise the real PingStream handler against an httptest.Server whose
// BaseContext mirrors what cmd/admin/main.go sets in production.
func TestSSE_ExitsOnBaseContextCancel(t *testing.T) {
	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	mux := http.NewServeMux()
	mux.HandleFunc("/stream", admin.PingStream)

	srv := httptest.NewUnstartedServer(mux)
	srv.Config.BaseContext = func(_ net.Listener) context.Context { return appCtx }
	srv.Start()
	defer srv.Close()

	// Stream client. Open the connection and start a goroutine that consumes
	// bytes until the body closes — we don't care about the events, just the
	// fact that the body returns EOF promptly after appCancel.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/stream", http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req) //nolint:bodyclose // body closed via the defer below; bodyclose can't track resp through the reader goroutine
	if err != nil {
		t.Fatalf("GET /stream: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d, want 200", resp.StatusCode)
	}

	bodyClosed := make(chan struct{})
	var bytesRead atomic.Int64
	go func() {
		defer close(bodyClosed)
		buf := make([]byte, 256)
		for {
			n, err := resp.Body.Read(buf)
			bytesRead.Add(int64(n))
			if err != nil {
				return
			}
		}
	}()

	// Give the handler a moment to emit its first tick — proves the stream
	// is live before we cancel.
	deadline := time.Now().Add(2 * time.Second)
	for bytesRead.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if bytesRead.Load() == 0 {
		t.Fatal("no bytes received from /stream before shutdown")
	}

	// Cancel the app ctx — equivalent to the SIGTERM cancel() in main.go.
	appCancel()

	select {
	case <-bodyClosed:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE handler did not exit within 2s after app ctx cancel — BaseContext probably not wired")
	}
}
