package metricsroll

import (
	"math"
	"sync"
	"testing"
	"time"
)

func t0() time.Time { return time.Unix(1700000000, 0) }

func nearly(a, b, eps float64) bool { return math.Abs(a-b) <= eps }

// CounterSlot --------------------------------------------------------------

func TestCounterSlotRateEmptyReturnsZero(t *testing.T) {
	s := NewCounterSlot(time.Minute, 0)
	if r := s.Rate(time.Minute); r != 0 {
		t.Fatalf("rate=%v, want 0", r)
	}
}

func TestCounterSlotRateOneSampleReturnsZero(t *testing.T) {
	s := NewCounterSlot(time.Minute, 0)
	s.Observe(t0(), 10)
	if r := s.Rate(time.Minute); r != 0 {
		t.Fatalf("rate=%v, want 0", r)
	}
}

func TestCounterSlotRateBasic(t *testing.T) {
	s := NewCounterSlot(time.Hour, 0)
	// 10 obs over 60s: value grows from 0 to 600. Rate = 10/s.
	for i := 0; i <= 10; i++ {
		s.Observe(t0().Add(time.Duration(i)*6*time.Second), float64(i*60))
	}
	got := s.Rate(time.Minute)
	if !nearly(got, 10, 0.001) {
		t.Fatalf("rate=%v, want ~10", got)
	}
}

func TestCounterSlotRateWindowExcludesOldSamples(t *testing.T) {
	s := NewCounterSlot(time.Hour, 0)
	s.Observe(t0(), 0)
	s.Observe(t0().Add(2*time.Minute), 1200) // outside 1m window from latest
	s.Observe(t0().Add(2*time.Minute+30*time.Second), 1500)
	// Within 1m window from latest: just sample 2 and 3, dt=30s, dv=300 → 10/s.
	got := s.Rate(time.Minute)
	if !nearly(got, 10, 0.001) {
		t.Fatalf("rate=%v, want ~10", got)
	}
}

func TestCounterSlotPrunesOutsideWindow(t *testing.T) {
	s := NewCounterSlot(time.Minute, 0)
	for i := 0; i < 5; i++ {
		s.Observe(t0().Add(time.Duration(i)*30*time.Second), float64(i))
	}
	// 5 samples spanning 0..120s, window=1m → only samples with ts >= 60s kept.
	// That's samples at 60s, 90s, 120s → 3.
	if got := s.Len(); got != 3 {
		t.Fatalf("len=%d, want 3", got)
	}
}

func TestCounterSlotRespectsCapacity(t *testing.T) {
	s := NewCounterSlot(time.Hour, 5)
	for i := 0; i < 10; i++ {
		s.Observe(t0().Add(time.Duration(i)*time.Second), float64(i))
	}
	if got := s.Len(); got != 5 {
		t.Fatalf("len=%d, want 5 (capacity)", got)
	}
}

func TestCounterSlotConcurrentObserve(t *testing.T) {
	s := NewCounterSlot(time.Hour, 0)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s.Observe(t0().Add(time.Duration(n)*time.Millisecond), float64(n))
		}(i)
	}
	wg.Wait()
	if s.Len() == 0 {
		t.Fatal("no samples recorded")
	}
}
