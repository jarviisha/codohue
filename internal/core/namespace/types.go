package namespace

import "time"

// Config holds the configuration for a single namespace.
type Config struct {
	// Core fields.
	Namespace     string             `json:"namespace"`
	ActionWeights map[string]float64 `json:"action_weights"`
	Lambda        float64            `json:"lambda"`
	Gamma         float64            `json:"gamma"`
	MaxResults    int                `json:"max_results"`
	SeenItemsDays int                `json:"seen_items_days"`

	// Auth.
	APIKeyHash string `json:"-"`

	// Dense hybrid.
	Alpha         float64 `json:"alpha"`
	DenseStrategy string  `json:"dense_strategy"`
	EmbeddingDim  int     `json:"embedding_dim"`
	DenseDistance string  `json:"dense_distance"`

	// Trending.
	TrendingWindow int     `json:"trending_window"`
	TrendingTTL    int     `json:"trending_ttl"`
	LambdaTrending float64 `json:"lambda_trending"`

	// Catalog auto-embedding (feature 004-catalog-embedder).
	// CatalogEnabled toggles whether this namespace accepts catalog ingest and
	// rejects BYOE writes for object dense vectors.
	CatalogEnabled bool `json:"catalog_enabled"`
	// CatalogStrategyID and CatalogStrategyVersion identify the active embedding
	// strategy registered in internal/core/embedstrategy. Both are empty when
	// CatalogEnabled is false.
	CatalogStrategyID      string         `json:"catalog_strategy_id,omitempty"`
	CatalogStrategyVersion string         `json:"catalog_strategy_version,omitempty"`
	CatalogStrategyParams  map[string]any `json:"catalog_strategy_params,omitempty"`
	CatalogMaxAttempts     int            `json:"catalog_max_attempts"`
	CatalogMaxContentBytes int            `json:"catalog_max_content_bytes"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
