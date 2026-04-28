package codohuetypes

import "time"

// RecommendedItem is a single recommendation with its relevance score and rank.
// Score is 0 for fallback paths (popular, trending cold-start) where no
// personalised relevance signal is available. Rank is 1-based global position
// accounting for the requested offset (rank = offset + i + 1).
type RecommendedItem struct {
	ObjectID string  `json:"object_id"`
	Score    float64 `json:"score"`
	Rank     int     `json:"rank"`
}

// Response is returned by the recommendations endpoint.
type Response struct {
	SubjectID   string            `json:"subject_id"`
	Namespace   string            `json:"namespace"`
	Items       []RecommendedItem `json:"items"`
	Source      string            `json:"source"`
	Limit       int               `json:"limit"`
	Offset      int               `json:"offset"`
	Total       int               `json:"total"`
	GeneratedAt time.Time         `json:"generated_at"`
}

// RankRequest is the payload for the rank endpoint.
type RankRequest struct {
	SubjectID  string   `json:"subject_id"`
	Namespace  string   `json:"namespace"`
	Candidates []string `json:"candidates"`
}

// RankedItem pairs an object ID with its computed relevance score and rank.
// Score is 0 when the subject has no interaction history (fallback — original
// order preserved). Rank is 1-based position in the response.
type RankedItem struct {
	ObjectID string  `json:"object_id"`
	Score    float64 `json:"score"`
	Rank     int     `json:"rank"`
}

// RankResponse is returned after ranking candidates.
type RankResponse struct {
	SubjectID   string       `json:"subject_id"`
	Namespace   string       `json:"namespace"`
	Items       []RankedItem `json:"items"`
	Source      string       `json:"source"`
	Total       int          `json:"total"`
	GeneratedAt time.Time    `json:"generated_at"`
}

// TrendingItem is a single item in the trending list with its score.
type TrendingItem struct {
	ObjectID string  `json:"object_id"`
	Score    float64 `json:"score"`
}

// TrendingResponse is returned by the trending endpoint.
type TrendingResponse struct {
	Namespace   string         `json:"namespace"`
	Items       []TrendingItem `json:"items"`
	WindowHours int            `json:"window_hours"`
	Limit       int            `json:"limit"`
	Offset      int            `json:"offset"`
	Total       int            `json:"total"`
	GeneratedAt time.Time      `json:"generated_at"`
}

// EmbeddingRequest is the payload for BYOE (bring-your-own-embedding) endpoints.
type EmbeddingRequest struct {
	Vector []float32 `json:"vector"`
}
