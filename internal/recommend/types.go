package recommend

import "github.com/jarviisha/codohue/pkg/codohuetypes"

// Request holds the query parameters for GET /v1/recommendations.
type Request struct {
	SubjectID string
	Namespace string
	Limit     int
}

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
