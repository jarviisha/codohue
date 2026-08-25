package redis

import "testing"

func TestTrendingKeyIsGenerationAware(t *testing.T) {
	if got := trendingKeyForGeneration("feed", 1); got != "trending:feed" {
		t.Fatalf("generation 1 = %q", got)
	}
	if got := trendingKeyForGeneration("feed", 3); got != "trending:feed:g3" {
		t.Fatalf("generation 3 = %q", got)
	}
}
