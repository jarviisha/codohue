package recommend

import (
	"math"
	"time"
)

func finiteScore(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func clampUnit(value float64) float64 {
	switch {
	case !finiteScore(value), value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
	}
}

func nonNegativeAgeDays(now, createdAt time.Time) float64 {
	age := now.Sub(createdAt).Hours() / 24
	if !finiteScore(age) || age < 0 {
		return 0
	}
	return age
}

func freshnessMultiplier(now, createdAt time.Time, gamma float64) float64 {
	if !finiteScore(gamma) || gamma < 0 {
		return 0
	}
	return clampUnit(math.Exp(-gamma * nonNegativeAgeDays(now, createdAt)))
}
