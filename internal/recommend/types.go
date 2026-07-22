package recommend

import "github.com/jarviisha/codohue/pkg/codohuetypes"

// Request holds the query parameters for subject recommendation reads.
type Request struct {
	SubjectID string
	Namespace string
	Limit     int
	Offset    int

	// degraded is set by the strategy paths when the response being served
	// is a fallback caused by an infrastructure error (Qdrant/Redis/DB
	// unavailable) rather than by the subject's data state (cold start).
	// Recommend skips caching degraded responses so a one-second blip does
	// not pin fallback results for the full cache TTL. Internal only.
	degraded bool
}

// RecommendedItem re-exports codohuetypes.RecommendedItem.
type RecommendedItem = codohuetypes.RecommendedItem

// Response re-exports codohuetypes.Response.
type Response = codohuetypes.Response

// EmbeddingRequest re-exports codohuetypes.EmbeddingRequest.
type EmbeddingRequest = codohuetypes.EmbeddingRequest

// RankRequest re-exports codohuetypes.RankRequest.
type RankRequest = codohuetypes.RankRequest

// RankedItem re-exports codohuetypes.RankedItem.
type RankedItem = codohuetypes.RankedItem

// RankResponse re-exports codohuetypes.RankResponse.
type RankResponse = codohuetypes.RankResponse

// TrendingItem re-exports codohuetypes.TrendingItem.
type TrendingItem = codohuetypes.TrendingItem

// TrendingResponse re-exports codohuetypes.TrendingResponse.
type TrendingResponse = codohuetypes.TrendingResponse

// Source constants
const (
	SourceCollaborativeFiltering = "collaborative_filtering"
	SourceFallbackPopular        = "fallback_popular"
	SourceHybridCold             = "hybrid_cold"
	SourceHybridRank             = "hybrid_rank"
	SourceHybrid                 = "hybrid" // sparse CF + dense blend
)
