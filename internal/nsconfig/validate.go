package nsconfig

import (
	"errors"
	"fmt"
	"math"
)

// Validation sentinels. The admin adapter maps these onto admin-side errors
// so the handler can pick 400/409/422 without importing this package.
var (
	// ErrInvalidConfig marks a request value outside its legal range → 400.
	ErrInvalidConfig = errors.New("nsconfig: invalid configuration")

	// ErrCatalogViaUpsert rejects dense_source="catalog" on the generic
	// upsert → 422. Catalog mode requires a strategy whose Dim matches
	// embedding_dim; only the catalog endpoint runs that validation, so
	// letting the generic PATCH flip the flag wedged the namespace: the
	// embedder dead-lettered every item at strategy_resolve while BYOE
	// writes started returning 409.
	ErrCatalogViaUpsert = errors.New("nsconfig: dense_source=catalog must be set via the catalog config endpoint")

	// ErrEmbeddingDimLocked rejects changing embedding_dim while dense
	// collections already exist → 409. Qdrant collections keep their
	// creation-time dimension and Ensure* only creates missing ones, so a
	// changed dim made every subsequent dense upsert fail on every cron
	// tick and embedder item, permanently, with no 400 anywhere.
	ErrEmbeddingDimLocked = errors.New("nsconfig: embedding_dim cannot change while dense collections exist; delete the namespace's dense collections (or the namespace) first")
)

// upsertDenseSources are the dense_source values the generic upsert accepts.
// "catalog" is deliberately absent — see ErrCatalogViaUpsert.
var upsertDenseSources = map[string]bool{
	"disabled": true,
	"item2vec": true,
	"svd":      true,
	"byoe":     true,
}

// denseDistances mirrors infra/qdrant's resolveDenseDistance vocabulary.
var denseDistances = map[string]bool{
	"cosine": true,
	"dot":    true,
}

// validateUpsert range-checks every supplied field. nil fields are PATCH
// no-ops and always pass.
func validateUpsert(req *UpsertRequest) error {
	if req == nil {
		return nil
	}
	// action_weights are deliberately unconstrained in sign: a negative
	// weight is the intended way to express a negative signal (SKIP/dislike
	// pushes an item away in the CF vector), so only NaN/Inf are rejected.
	for action, w := range req.ActionWeights {
		if math.IsNaN(w) || math.IsInf(w, 0) {
			return fmt.Errorf("%w: action_weights[%s] must be a finite number, got %v", ErrInvalidConfig, action, w)
		}
	}
	if req.Lambda != nil && *req.Lambda <= 0 {
		return fmt.Errorf("%w: lambda must be > 0, got %v", ErrInvalidConfig, *req.Lambda)
	}
	if req.Gamma != nil && *req.Gamma < 0 {
		return fmt.Errorf("%w: gamma must be >= 0, got %v", ErrInvalidConfig, *req.Gamma)
	}
	if req.MaxResults != nil && *req.MaxResults <= 0 {
		return fmt.Errorf("%w: max_results must be > 0, got %d", ErrInvalidConfig, *req.MaxResults)
	}
	if req.SeenItemsDays != nil && *req.SeenItemsDays <= 0 {
		return fmt.Errorf("%w: seen_items_days must be > 0, got %d", ErrInvalidConfig, *req.SeenItemsDays)
	}
	if req.Alpha != nil && (*req.Alpha < 0 || *req.Alpha > 1) {
		// Out-of-range alpha silently disabled hybrid blending instead of
		// erroring — the recommend service only blends for 0 < alpha < 1.
		return fmt.Errorf("%w: alpha must be within [0, 1], got %v", ErrInvalidConfig, *req.Alpha)
	}
	if req.DenseSource != nil {
		if *req.DenseSource == "catalog" {
			return ErrCatalogViaUpsert
		}
		if !upsertDenseSources[*req.DenseSource] {
			return fmt.Errorf("%w: dense_source must be one of disabled|item2vec|svd|byoe, got %q", ErrInvalidConfig, *req.DenseSource)
		}
	}
	if req.EmbeddingDim != nil && *req.EmbeddingDim <= 0 {
		return fmt.Errorf("%w: embedding_dim must be > 0, got %d", ErrInvalidConfig, *req.EmbeddingDim)
	}
	if req.DenseDistance != nil && *req.DenseDistance != "" && !denseDistances[*req.DenseDistance] {
		return fmt.Errorf("%w: dense_distance must be one of cosine|dot, got %q", ErrInvalidConfig, *req.DenseDistance)
	}
	if req.TrendingWindow != nil && *req.TrendingWindow <= 0 {
		return fmt.Errorf("%w: trending_window must be > 0, got %d", ErrInvalidConfig, *req.TrendingWindow)
	}
	if req.TrendingTTL != nil && *req.TrendingTTL <= 0 {
		return fmt.Errorf("%w: trending_ttl must be > 0, got %d", ErrInvalidConfig, *req.TrendingTTL)
	}
	if req.LambdaTrending != nil && *req.LambdaTrending <= 0 {
		return fmt.Errorf("%w: lambda_trending must be > 0, got %v", ErrInvalidConfig, *req.LambdaTrending)
	}
	return nil
}
