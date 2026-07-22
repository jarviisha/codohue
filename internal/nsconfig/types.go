package nsconfig

import (
	"fmt"
	"time"
)

// UpsertRequest is the payload for creating or updating a namespace config.
//
// Every field is a pointer with PATCH semantics: nil means "leave this column
// alone". A value-typed field cannot express that, and the admin UI submits
// only the fields the operator actually edited — so value fields made a
// one-field edit silently reset every other column to its Go zero value.
// On create, nil fields fall through to the column defaults in the schema.
type UpsertRequest struct {
	ActionWeights map[string]float64 `json:"action_weights,omitempty"`
	Lambda        *float64           `json:"lambda,omitempty"`
	Gamma         *float64           `json:"gamma,omitempty"`
	MaxResults    *int               `json:"max_results,omitempty"`
	SeenItemsDays *int               `json:"seen_items_days,omitempty"`

	// ExcludeAuthored drops the subject's own authored objects from their
	// recommendations. Defaults to false — see migration 020.
	ExcludeAuthored *bool `json:"exclude_authored,omitempty"`

	// Dense hybrid
	Alpha         *float64 `json:"alpha,omitempty"`
	DenseSource   *string  `json:"dense_source,omitempty"`
	EmbeddingDim  *int     `json:"embedding_dim,omitempty"`
	DenseDistance *string  `json:"dense_distance,omitempty"`

	// Trending
	TrendingWindow *int     `json:"trending_window,omitempty"`
	TrendingTTL    *int     `json:"trending_ttl,omitempty"`
	LambdaTrending *float64 `json:"lambda_trending,omitempty"`
}

// UpsertResponse is returned after a successful upsert.
type UpsertResponse struct {
	Namespace string    `json:"namespace"`
	UpdatedAt time.Time `json:"updated_at"`
	// APIKey is the plaintext API key returned only on initial namespace creation.
	// It will not appear on subsequent updates.
	APIKey string `json:"api_key,omitempty"`
}

// RotateAPIKeyResponse carries the freshly minted namespace key. Like
// creation, the plaintext appears exactly once — only its bcrypt hash is
// stored.
type RotateAPIKeyResponse struct {
	Namespace string `json:"namespace"`
	APIKey    string `json:"api_key"`
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

// ptr returns a pointer to v. UpsertRequest fields are pointers so that nil
// can mean "leave this column alone"; this keeps literals readable.
func ptr[T any](v T) *T { return &v }
