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

// Age is measured in whole days from a timestamp the client controls. A future
// creation time would make it negative, and e^(-γ·negative) > 1 — a boost.
// Validation rejects those at the boundary, but scoring must stay correct for
// rows already stored before that rule existed.
func TestNonNegativeAgeDays(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name      string
		createdAt time.Time
		want      float64
	}{
		{"two days old", now.Add(-48 * time.Hour), 2},
		{"same instant", now, 0},
		{"future is floored at zero", now.Add(72 * time.Hour), 0},
		{"zero value is not negative", time.Time{}, -1}, // sentinel: only checked for >= 0
	} {
		got := nonNegativeAgeDays(now, tc.createdAt)
		if got < 0 {
			t.Errorf("%s: age %v is negative", tc.name, got)
		}
		if tc.want >= 0 && got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A zero/absent timestamp must not blow the multiplier up: the year-1 zero
// value is ~740k days old, and e^(-γ·740000) underflows to 0 rather than
// producing NaN.
func TestFreshnessMultiplier_MalformedTimestampsStayFinite(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name      string
		createdAt time.Time
		gamma     float64
	}{
		{"zero timestamp", time.Time{}, 0.1},
		{"far future", now.AddDate(10000, 0, 0), 0.1},
		{"far past", now.AddDate(-10000, 0, 0), 0.1},
		{"gamma zero disables decay", now.Add(-100 * 24 * time.Hour), 0},
		{"negative gamma is refused", now, -1},
		{"NaN gamma is refused", now, math.NaN()},
	} {
		got := freshnessMultiplier(now, tc.createdAt, tc.gamma)
		if !finiteScore(got) || got < 0 || got > 1 {
			t.Errorf("%s: multiplier %v is not a finite value in [0,1]", tc.name, got)
		}
	}
	// γ=0 means "no freshness decay", which must be a neutral multiplier
	// rather than a silent zeroing of every score.
	if got := freshnessMultiplier(now, now.Add(-100*24*time.Hour), 0); got != 1 {
		t.Errorf("gamma=0 multiplier: got %v, want 1", got)
	}
}

// finiteScore is the gate every candidate passes before it can reach JSON.
// encoding/json cannot represent NaN or ±Inf, so a single leaked value fails
// the whole response, not just that item.
func TestFiniteScore_RejectsUnserializableValues(t *testing.T) {
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if finiteScore(value) {
			t.Errorf("finiteScore(%v) must be false — encoding/json cannot marshal it", value)
		}
	}
	for _, value := range []float64{0, -1, 1, math.MaxFloat64, -math.MaxFloat64} {
		if !finiteScore(value) {
			t.Errorf("finiteScore(%v) must be true", value)
		}
	}
}
