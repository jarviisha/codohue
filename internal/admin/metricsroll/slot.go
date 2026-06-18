package metricsroll

import (
	"sort"
	"sync"
	"time"
)

// CounterSlot stores cumulative counter snapshots over a sliding window and
// computes events-per-second across that window.
//
// Typical use: a goroutine reads a Prometheus counter every 10s and calls
// Observe(now, value). Handlers call Rate(1*time.Minute) to surface a 1-minute
// rate in `/metrics/summary`.
type CounterSlot struct {
	mu       sync.Mutex
	samples  []counterSample
	capacity int
	window   time.Duration
}

type counterSample struct {
	ts    time.Time
	value float64
}

// NewCounterSlot returns a slot retaining samples up to window. capacity caps
// in-memory samples (oldest evicted first) and defaults to 1024 when <= 0.
func NewCounterSlot(window time.Duration, capacity int) *CounterSlot {
	if capacity <= 0 {
		capacity = 1024
	}
	return &CounterSlot{
		capacity: capacity,
		window:   window,
		samples:  make([]counterSample, 0, capacity),
	}
}

// Observe records a counter's cumulative value at ts.
func (s *CounterSlot) Observe(ts time.Time, value float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.samples = append(s.samples, counterSample{ts, value})
	s.prune(ts)
}

// Rate returns events per second across the most recent window. Returns 0
// when fewer than two samples fall inside the window or the sample span is
// zero.
func (s *CounterSlot) Rate(window time.Duration) float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.samples) < 2 {
		return 0
	}
	latest := s.samples[len(s.samples)-1]
	cutoff := latest.ts.Add(-window)
	var firstIdx = -1
	for i := range s.samples {
		if !s.samples[i].ts.Before(cutoff) {
			firstIdx = i
			break
		}
	}
	if firstIdx < 0 || firstIdx == len(s.samples)-1 {
		return 0
	}
	first := s.samples[firstIdx]
	dt := latest.ts.Sub(first.ts).Seconds()
	if dt <= 0 {
		return 0
	}
	return (latest.value - first.value) / dt
}

// Len reports the current sample count (after prune). Test-only helper.
func (s *CounterSlot) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.samples)
}

func (s *CounterSlot) prune(now time.Time) {
	cutoff := now.Add(-s.window)
	idx := 0
	for i := range s.samples {
		if !s.samples[i].ts.Before(cutoff) {
			break
		}
		idx = i + 1
	}
	if idx > 0 {
		n := copy(s.samples, s.samples[idx:])
		s.samples = s.samples[:n]
	}
	if len(s.samples) > s.capacity {
		excess := len(s.samples) - s.capacity
		n := copy(s.samples, s.samples[excess:])
		s.samples = s.samples[:n]
	}
}

// HistogramSlot stores raw observations and computes percentiles from samples
// inside the configured window. Currently uses sort-on-read; swap to a
// t-digest sketch if a hot path observes thousands of samples per second.
type HistogramSlot struct {
	mu       sync.Mutex
	samples  []histSample
	capacity int
	window   time.Duration
}

type histSample struct {
	ts    time.Time
	value float64
}

// NewHistogramSlot returns a slot retaining samples up to window. capacity
// defaults to 10 000 when <= 0.
func NewHistogramSlot(window time.Duration, capacity int) *HistogramSlot {
	if capacity <= 0 {
		capacity = 10_000
	}
	return &HistogramSlot{
		capacity: capacity,
		window:   window,
		samples:  make([]histSample, 0, capacity),
	}
}

// Observe records a single observation at ts.
func (s *HistogramSlot) Observe(ts time.Time, value float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.samples = append(s.samples, histSample{ts, value})
	s.prune(ts)
}

// Percentile returns the p-th percentile (0..1) of observations inside the
// most recent window. Returns 0 when the slot is empty or p is out of range.
func (s *HistogramSlot) Percentile(p float64) float64 {
	if p < 0 || p > 1 {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.samples) == 0 {
		return 0
	}
	cutoff := s.samples[len(s.samples)-1].ts.Add(-s.window)
	vals := make([]float64, 0, len(s.samples))
	for _, sm := range s.samples {
		if !sm.ts.Before(cutoff) {
			vals = append(vals, sm.value)
		}
	}
	if len(vals) == 0 {
		return 0
	}
	sort.Float64s(vals)
	idx := int(p * float64(len(vals)-1))
	if idx < 0 {
		idx = 0
	}
	if idx >= len(vals) {
		idx = len(vals) - 1
	}
	return vals[idx]
}

// Len reports the current sample count (after prune). Test-only helper.
func (s *HistogramSlot) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.samples)
}

func (s *HistogramSlot) prune(now time.Time) {
	cutoff := now.Add(-s.window)
	idx := 0
	for i := range s.samples {
		if !s.samples[i].ts.Before(cutoff) {
			break
		}
		idx = i + 1
	}
	if idx > 0 {
		n := copy(s.samples, s.samples[idx:])
		s.samples = s.samples[:n]
	}
	if len(s.samples) > s.capacity {
		excess := len(s.samples) - s.capacity
		n := copy(s.samples, s.samples[excess:])
		s.samples = s.samples[:n]
	}
}
