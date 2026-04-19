package nsconfig

import "time"

// NamespaceConfig holds the configuration for a single namespace.
type NamespaceConfig struct {
	// Core fields — from MVP
	Namespace     string             `json:"namespace"`
	ActionWeights map[string]float64 `json:"action_weights"`
	Lambda        float64            `json:"lambda"`
	Gamma         float64            `json:"gamma"`
	MaxResults    int                `json:"max_results"`
	SeenItemsDays int                `json:"seen_items_days"`

	// Auth — Phase 2
	APIKeyHash string `json:"-"` // bcrypt hash; never serialized

	// Dense hybrid — Phase 3
	Alpha         float64 `json:"alpha"`          // weight of sparse CF score (default 0.7)
	DenseStrategy string  `json:"dense_strategy"` // "item2vec" | "svd" | "byoe" | "disabled"
	EmbeddingDim  int     `json:"embedding_dim"`  // dense vector dimensions (default 64)
	DenseDistance string  `json:"dense_distance"` // "cosine" | "dot" (default "cosine")

	// Trending — Phase 3
	TrendingWindow int     `json:"trending_window"` // hours to look back (default 24)
	TrendingTTL    int     `json:"trending_ttl"`    // Redis ZSET TTL in seconds (default 600)
	LambdaTrending float64 `json:"lambda_trending"` // decay rate for trending (default 0.1)

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UpsertRequest is the payload from the Main Backend for creating or updating a namespace config.
type UpsertRequest struct {
	ActionWeights map[string]float64 `json:"action_weights"`
	Lambda        float64            `json:"lambda"`
	Gamma         float64            `json:"gamma"`
	MaxResults    int                `json:"max_results"`
	SeenItemsDays int                `json:"seen_items_days"`

	// Dense hybrid
	Alpha         float64 `json:"alpha"`
	DenseStrategy string  `json:"dense_strategy"`
	EmbeddingDim  int     `json:"embedding_dim"`
	DenseDistance string  `json:"dense_distance"`

	// Trending
	TrendingWindow int     `json:"trending_window"`
	TrendingTTL    int     `json:"trending_ttl"`
	LambdaTrending float64 `json:"lambda_trending"`
}

// UpsertResponse is returned after a successful upsert.
type UpsertResponse struct {
	Namespace string    `json:"namespace"`
	UpdatedAt time.Time `json:"updated_at"`
	// APIKey is the plaintext API key returned only on initial namespace creation.
	// It will not appear on subsequent updates.
	APIKey string `json:"api_key,omitempty"`
}
