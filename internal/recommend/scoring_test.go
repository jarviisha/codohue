package recommend

import (
	"math"
	"testing"
	"time"
)

func TestFreshnessMultiplierIsFiniteAndClamped(t *testing.T) {
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	for name, createdAt := range map[string]time.Time{
		"past":    now.Add(-24 * time.Hour),
		"present": now,
		"future":  now.Add(24 * time.Hour),
	} {
		got := freshnessMultiplier(now, createdAt, 0.1)
		if !finiteScore(got) || got < 0 || got > 1 {
			t.Fatalf("%s multiplier=%v", name, got)
		}
	}
	if got := freshnessMultiplier(now, now, math.Inf(1)); got != 0 {
		t.Fatalf("non-finite gamma multiplier=%v, want 0", got)
	}
}

func TestClampUnitRejectsNonFiniteValues(t *testing.T) {
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -1} {
		if got := clampUnit(value); got != 0 {
			t.Fatalf("clampUnit(%v)=%v", value, got)
		}
	}
	if got := clampUnit(2); got != 1 {
		t.Fatalf("clampUnit(2)=%v", got)
	}
}
