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

// RankRequest is the payload for the rank endpoint. The namespace is supplied
// via the URL path (/v1/namespaces/{ns}/rankings) and is no longer carried in
// the body.
type RankRequest struct {
	SubjectID  string   `json:"subject_id"`
	Candidates []string `json:"candidates"`
}

// RankedItem pairs an object ID with its computed relevance score and rank.
// Rank is 1-based position in the response.
//
// Scored distinguishes the two reasons a candidate can come back: true means
// the engine evaluated it (a low or zero score is a real relevance verdict),
// false means it was returned unscored so the caller's candidate list comes
// back whole — the subject has no vector at all (see the response Source),
// the candidate was never indexed, or an eligibility filter excluded it.
type RankedItem struct {
	ObjectID string  `json:"object_id"`
	Score    float64 `json:"score"`
	Rank     int     `json:"rank"`
	Scored   bool    `json:"scored"`
}

// RankResponse is returned after ranking candidates.
//
// Source is "hybrid_rank" when the subject was scored, or
// "no_subject_vector" when the subject has no sparse and no dense vector —
// every item then carries Scored=false and the original request order.
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

	// ObjectCreatedAt optionally records when the object was created so the
	// γ-freshness rerank can decay it like sparse-path items. Only read by
	// the object-embedding endpoint; ignored for subject embeddings.
	ObjectCreatedAt *time.Time `json:"object_created_at,omitempty"`
}
