package compute

import (
	"math"
	"time"
)

// TrendingScores aggregates events within the given window and returns a
// trending score per object.
//
// Formula:
//
//	score(item) = Σ  action_weight[a] × e^(-λ_trending × age_hours(event))
//	              ∀ events of item within the last windowHours
//
// Events older than windowHours are ignored.
// If actionWeights is nil or does not contain an action, weight defaults to 1.0.
//
// Note on action_weights: the same per-namespace weights used for CF sparse vectors are
// applied here. This means high-weight actions (e.g. SHARE=10, LIKE=5) dominate the
// trending ranking — an item with 10 shares outscores an item with 100 views when
// VIEW=1. This is intentional: namespaces where engagement signals matter more than
// raw consumption should keep their weights as-is. Namespaces that want view-count-style
// trending (all actions equal) should set all action_weights to 1.0.
func TrendingScores(events []*RawEvent, actionWeights map[string]float64, lambdaTrending float64, windowHours int) map[string]float64 {
	if len(events) == 0 {
		return nil
	}

	now := time.Now().Unix()
	windowSecs := int64(windowHours) * 3600

	scores := make(map[string]float64)
	for _, e := range events {
		if now-e.OccurredAt > windowSecs {
			continue
		}

		weight := 1.0
		if w, ok := actionWeights[e.Action]; ok {
			weight = w
		}

		ageHours := float64(now-e.OccurredAt) / 3600.0
		scores[e.ObjectID] += weight * math.Exp(-lambdaTrending*ageHours)
	}
	return scores
}
