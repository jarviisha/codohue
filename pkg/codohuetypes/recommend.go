package codohuetypes

import "time"

// Response is returned by the recommendations endpoint.
type Response struct {
	SubjectID   string    `json:"subject_id"`
	Namespace   string    `json:"namespace"`
	Items       []string  `json:"items"`
	Source      string    `json:"source"`
	GeneratedAt time.Time `json:"generated_at"`
}

// RankRequest is the payload for the rank endpoint.
type RankRequest struct {
	SubjectID  string   `json:"subject_id"`
	Namespace  string   `json:"namespace"`
	Candidates []string `json:"candidates"`
}

// RankedItem pairs an object ID with its computed relevance score.
// Score is 0 when the subject has no interaction history (fallback — original
// order preserved).
type RankedItem struct {
	ObjectID string  `json:"object_id"`
	Score    float64 `json:"score"`
}

// RankResponse is returned after ranking candidates.
type RankResponse struct {
	SubjectID   string       `json:"subject_id"`
	Namespace   string       `json:"namespace"`
	Items       []RankedItem `json:"items"`
	Source      string       `json:"source"`
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
	GeneratedAt time.Time      `json:"generated_at"`
}

// EmbeddingRequest is the payload for BYOE (bring-your-own-embedding) endpoints.
type EmbeddingRequest struct {
	Vector []float32 `json:"vector"`
}
