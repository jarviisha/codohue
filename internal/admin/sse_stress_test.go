package admin

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jarviisha/codohue/internal/admin/eventbus"
)

// TestSSEStress_100Clients_1kEventsPerSec pins the BUILD_PLAN §12.2
// performance budget: a single replica must hold ≥100 concurrent SSE
// connections while events fan out at ~1 000/s with p95 event-to-client
// latency under one second.
//
// The test runs the real streamRun loop behind an httptest.Server whose
// BaseContext mirrors cmd/admin's wiring. Publisher pushes timestamped
// events through the real eventbus; clients parse the SSE frames they
// receive and compute the wall-clock delta. Skipped in -short mode so it
// doesn't slow regular runs.
func TestSSEStress_100Clients_1kEventsPerSec(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}

	const (
		nClients     = 100
		eventsPerSec = 1000
		duration     = 3 * time.Second
		drainGrace   = 1 * time.Second
		// Generous buffer — with 100 fast loopback consumers the per-subscriber
		// channel never fills, but we want to assert this stays true under
		// the spec workload rather than rely on default sizing.
		busBufferSize = 4096
		// Latency budget per BUILD_PLAN §12.2.
		p95Budget = 1 * time.Second
		// Coverage budget: each client should see at least this fraction of
		// the published events. 95% allows for connect-race losses before
		// the publisher's first tick lands.
		coverageBudget = 0.95
	)

	bus := eventbus.NewBus(eventbus.WithBufferSize(busBufferSize))
	defer bus.Close()

	appCtx, appCancel := context.WithCancel(context.Background())
	defer appCancel()

	mux := http.NewServeMux()
	mux.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		events, cancel := bus.Subscribe(eventbus.Filter{})
		defer cancel()
		streamRun(w, r, events, "stress")
	})

	srv := httptest.NewUnstartedServer(mux)
	srv.Config.BaseContext = func(_ net.Listener) context.Context { return appCtx }
	srv.Start()
	defer srv.Close()

	// Each client tracks its own counters + latencies to avoid cross-goroutine
	// contention on a shared slice (this matters at ~100k samples/s).
	type clientStats struct {
		received  int64
		latencies []time.Duration
		connectOK bool
		err       error
	}
	stats := make([]clientStats, nClients)

	var connectedWg sync.WaitGroup
	var clientsWg sync.WaitGroup
	for i := 0; i < nClients; i++ {
		connectedWg.Add(1)
		clientsWg.Add(1)
		go func(idx int) {
			defer clientsWg.Done()
			st := &stats[idx]
			// Reuse a single client per goroutine with no overall timeout —
			// the test bounds wall time via appCancel below.
			httpClient := &http.Client{}
			req, err := http.NewRequestWithContext(context.Background(), "GET", srv.URL+"/stream", http.NoBody)
			if err != nil {
				st.err = err
				connectedWg.Done()
				return
			}
			resp, err := httpClient.Do(req)
			if err != nil {
				st.err = err
				connectedWg.Done()
				return
			}
			defer resp.Body.Close()
			st.connectOK = true
			connectedWg.Done()

			scanner := bufio.NewScanner(resp.Body)
			scanner.Buffer(make([]byte, 0, 8*1024), 64*1024)

			st.latencies = make([]time.Duration, 0, int(duration.Seconds())*eventsPerSec)

			for scanner.Scan() {
				line := scanner.Text()
				const prefix = "data: "
				if !strings.HasPrefix(line, prefix) {
					continue
				}
				var ev struct {
					SentNs int64 `json:"sent_ns"`
				}
				if err := json.Unmarshal([]byte(line[len(prefix):]), &ev); err != nil {
					continue
				}
				if ev.SentNs > 0 {
					st.latencies = append(st.latencies, time.Duration(time.Now().UnixNano()-ev.SentNs))
					atomic.AddInt64(&st.received, 1)
				}
			}
		}(i)
	}

	// Wait for every client to finish the HTTP handshake before the publisher
	// starts firing — otherwise late connectors miss the early events and
	// dominate the loss numbers.
	connectedWg.Wait()
	for i, s := range stats {
		if !s.connectOK {
			t.Fatalf("client %d failed to connect: %v", i, s.err)
		}
	}
	// One extra grace period so each handler has subscribed to the bus
	// (Subscribe is the very first call inside the handler).
	time.Sleep(100 * time.Millisecond)

	// Publisher: tick at eventsPerSec for `duration`. We measure actual sent
	// count rather than computing it from rate × duration so an under-tick
	// (ticker drift on a busy host) doesn't fail the coverage assertion.
	pubCtx, pubCancel := context.WithTimeout(context.Background(), duration)
	defer pubCancel()

	var sent int64
	tickInterval := time.Second / eventsPerSec
	pubTicker := time.NewTicker(tickInterval)
	defer pubTicker.Stop()
PUBLISH:
	for {
		select {
		case <-pubCtx.Done():
			break PUBLISH
		case <-pubTicker.C:
			bus.Publish(context.Background(), eventbus.Event{
				Kind:    "stress.tick",
				Payload: map[string]any{"sent_ns": time.Now().UnixNano()},
			})
			atomic.AddInt64(&sent, 1)
		}
	}

	// Drain grace lets in-flight buffers flush before we cancel handlers.
	time.Sleep(drainGrace)
	appCancel() // unblocks streamRun via BaseContext

	// Each handler should exit within a small budget once appCtx is cancelled.
	done := make(chan struct{})
	go func() { clientsWg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("clients did not finish reading within 5s of app cancel")
	}

	// Aggregate
	totalSent := atomic.LoadInt64(&sent)
	if totalSent == 0 {
		t.Fatal("publisher emitted zero events")
	}
	totalReceived := int64(0)
	minRecv := int64(1<<63 - 1)
	allLatencies := make([]time.Duration, 0, nClients*int(totalSent))
	for _, s := range stats {
		n := atomic.LoadInt64(&s.received)
		totalReceived += n
		if n < minRecv {
			minRecv = n
		}
		allLatencies = append(allLatencies, s.latencies...)
	}

	threshold := int64(float64(totalSent) * coverageBudget)
	if minRecv < threshold {
		t.Errorf("slowest client got %d events; want >= %d (%.0f%% of %d sent)",
			minRecv, threshold, coverageBudget*100, totalSent)
	}

	if len(allLatencies) == 0 {
		t.Fatal("no latency samples collected")
	}
	sort.Slice(allLatencies, func(i, j int) bool { return allLatencies[i] < allLatencies[j] })
	p50 := allLatencies[len(allLatencies)*50/100]
	p95 := allLatencies[len(allLatencies)*95/100]
	p99 := allLatencies[len(allLatencies)*99/100]
	if p95 > p95Budget {
		t.Errorf("p95 event-to-client latency = %v; budget %v (§12.2)", p95, p95Budget)
	}

	t.Logf(
		"stress: clients=%d sent=%d total_received=%d min_per_client=%d p50=%v p95=%v p99=%v",
		nClients, totalSent, totalReceived, minRecv, p50, p95, p99,
	)
}
