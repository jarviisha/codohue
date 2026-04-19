package compute

import (
	"math"
	"testing"
	"time"
)

// TestTrendingScoresBasic verifies that a recent event produces a positive score.
func TestTrendingScoresBasic(t *testing.T) {
	now := time.Now().Unix()
	events := []*RawEvent{
		{ObjectID: "obj1", Action: "like", Weight: 5, OccurredAt: now - 3600}, // 1h ago
	}
	weights := map[string]float64{"like": 5.0}
	scores := TrendingScores(events, weights, 0.1, 24)

	if scores == nil {
		t.Fatal("expected non-nil scores")
	}
	s, ok := scores["obj1"]
	if !ok {
		t.Fatal("expected obj1 in scores")
	}
	if s <= 0 {
		t.Errorf("expected positive score, got %f", s)
	}
}

// TestTrendingScoresWindowExclusion verifies that events outside the window are ignored.
func TestTrendingScoresWindowExclusion(t *testing.T) {
	now := time.Now().Unix()
	events := []*RawEvent{
		{ObjectID: "obj-old", Action: "view", Weight: 1, OccurredAt: now - 48*3600}, // 48h ago
		{ObjectID: "obj-new", Action: "view", Weight: 1, OccurredAt: now - 3600},    // 1h ago
	}
	scores := TrendingScores(events, nil, 0.1, 24) // 24h window

	if _, ok := scores["obj-old"]; ok {
		t.Error("obj-old should be excluded (outside 24h window)")
	}
	if _, ok := scores["obj-new"]; !ok {
		t.Error("obj-new should be included (inside 24h window)")
	}
}

// TestTrendingScoresDecay verifies that fresher events score higher than older ones.
func TestTrendingScoresDecay(t *testing.T) {
	now := time.Now().Unix()
	events := []*RawEvent{
		{ObjectID: "obj-fresh", Action: "like", Weight: 1, OccurredAt: now - 3600},    // 1h ago
		{ObjectID: "obj-stale", Action: "like", Weight: 1, OccurredAt: now - 20*3600}, // 20h ago
	}
	scores := TrendingScores(events, map[string]float64{"like": 1.0}, 0.1, 24)

	if scores["obj-fresh"] <= scores["obj-stale"] {
		t.Errorf("fresh item should score higher: fresh=%f stale=%f",
			scores["obj-fresh"], scores["obj-stale"])
	}
}

// TestTrendingScoresDefaultWeight verifies that unknown actions default to weight 1.
func TestTrendingScoresDefaultWeight(t *testing.T) {
	now := time.Now().Unix()
	events := []*RawEvent{
		{ObjectID: "obj1", Action: "unknown_action", Weight: 0, OccurredAt: now - 60},
	}
	scores := TrendingScores(events, nil, 0.0, 24) // lambda=0 → no decay

	s, ok := scores["obj1"]
	if !ok {
		t.Fatal("expected obj1 in scores")
	}
	// With lambda=0 and weight=1: score = 1.0 * e^0 = 1.0
	if math.Abs(s-1.0) > 1e-6 {
		t.Errorf("expected score ~1.0, got %f", s)
	}
}

// TestTrendingScoresAggregation verifies that multiple events for the same object accumulate.
func TestTrendingScoresAggregation(t *testing.T) {
	now := time.Now().Unix()
	events := []*RawEvent{
		{ObjectID: "obj1", Action: "view", Weight: 1, OccurredAt: now - 100},
		{ObjectID: "obj1", Action: "view", Weight: 1, OccurredAt: now - 200},
		{ObjectID: "obj2", Action: "view", Weight: 1, OccurredAt: now - 100},
	}
	scores := TrendingScores(events, map[string]float64{"view": 1.0}, 0.0, 24) // no decay

	// obj1 gets two events; obj2 gets one
	if scores["obj1"] <= scores["obj2"] {
		t.Errorf("obj1 should score higher (2 events vs 1): obj1=%f obj2=%f",
			scores["obj1"], scores["obj2"])
	}
}

// TestTrendingScoresEmpty verifies that nil/empty input returns nil.
func TestTrendingScoresEmpty(t *testing.T) {
	if result := TrendingScores(nil, nil, 0.1, 24); result != nil {
		t.Errorf("expected nil for nil events, got %v", result)
	}
	if result := TrendingScores([]*RawEvent{}, nil, 0.1, 24); result != nil {
		t.Errorf("expected nil for empty events, got %v", result)
	}
}
