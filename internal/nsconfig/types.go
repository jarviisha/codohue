package nsconfig

import (
	"fmt"
	"time"
)

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

// UpdateCatalogRequest is the payload accepted by Service.UpdateCatalogConfig.
// It carries only the catalog-specific fields so the catalog admin endpoint
// stays orthogonal to the existing UpsertRequest path.
type UpdateCatalogRequest struct {
	Enabled         bool           `json:"enabled"`
	StrategyID      string         `json:"strategy_id,omitempty"`
	StrategyVersion string         `json:"strategy_version,omitempty"`
	Params          map[string]any `json:"params,omitempty"`
	MaxAttempts     int            `json:"max_attempts,omitempty"`
	MaxContentBytes int            `json:"max_content_bytes,omitempty"`
}

// DimensionMismatchError is returned by Service.UpdateCatalogConfig when the
// chosen strategy version produces a vector at a dimension that does not
// equal the namespace's existing embedding_dim. It carries both numbers so
// the admin handler can render them in the error body verbatim (US2 #2).
type DimensionMismatchError struct {
	StrategyDim           int
	NamespaceEmbeddingDim int
}

func (e *DimensionMismatchError) Error() string {
	return fmt.Sprintf("strategy dimension mismatch: strategy_dim=%d namespace_embedding_dim=%d",
		e.StrategyDim, e.NamespaceEmbeddingDim)
}
