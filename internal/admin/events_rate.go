package admin

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/jarviisha/codohue/internal/admin/metricsroll"
)

// eventRateRetention is how far back each namespace's CounterSlot keeps
// samples. One hour covers the 1m/5m rates surfaced in /metrics/summary while
// bounding memory (a 10s sampler yields ~360 samples per namespace).
const eventRateRetention = time.Hour

// EventRateTracker estimates per-namespace ingest rates from a stream of
// Observe calls. The events-tail bridge calls Observe once per event it sees
// on Redis pub/sub; a sampler goroutine (Run) snapshots the running counts
// into rolling windows. Handlers read RatePerSec / RatesPerSec to surface
// fleet events/min and /metrics/summary without touching the events table or
// scraping cmd/api's Prometheus endpoint. Safe for concurrent use. Satisfies
// the service's eventRateReader interface.
type EventRateTracker struct {
	mu sync.Mutex
	ns map[string]*nsRate
}

type nsRate struct {
	count uint64 // cumulative events observed for this namespace
	slot  *metricsroll.CounterSlot
}

// NewEventRateTracker returns a ready tracker.
func NewEventRateTracker() *EventRateTracker {
	return &EventRateTracker{ns: make(map[string]*nsRate)}
}

// Observe records one ingested event for namespace.
func (t *EventRateTracker) Observe(namespace string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.ns[namespace]
	if r == nil {
		r = &nsRate{slot: metricsroll.NewCounterSlot(eventRateRetention, 0)}
		t.ns[namespace] = r
	}
	r.count++
}

// Sample snapshots every namespace's cumulative count into its rolling window
// at ts. Call on a fixed interval so Rate has at least two samples to diff.
func (t *EventRateTracker) Sample(ts time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, r := range t.ns {
		r.slot.Observe(ts, float64(r.count))
	}
}

// RatePerSec returns the events-per-second rate for namespace across window,
// or 0 when the namespace is unknown or has too few samples.
func (t *EventRateTracker) RatePerSec(namespace string, window time.Duration) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	r := t.ns[namespace]
	if r == nil {
		return 0
	}
	return r.slot.Rate(window)
}

// RatesPerSec returns the rate across window for every namespace seen so far.
func (t *EventRateTracker) RatesPerSec(window time.Duration) map[string]float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make(map[string]float64, len(t.ns))
	for ns, r := range t.ns {
		out[ns] = r.slot.Rate(window)
	}
	return out
}

// Namespaces returns the namespaces observed so far, sorted. Test/diagnostic helper.
func (t *EventRateTracker) Namespaces() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, 0, len(t.ns))
	for ns := range t.ns {
		out = append(out, ns)
	}
	sort.Strings(out)
	return out
}

// Run drives Sample on an interval ticker until ctx is cancelled. Spawn in a
// goroutine during wiring.
func (t *EventRateTracker) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			t.Sample(now)
		}
	}
}
