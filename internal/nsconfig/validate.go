package nsconfig

import (
	"errors"
	"fmt"
	"math"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// Validation sentinels. The admin adapter maps these onto admin-side errors
// so the handler can pick 400/409/422 without importing this package.
var (
	// ErrInvalidConfig marks a request value outside its legal range → 400.
	ErrInvalidConfig = errors.New("nsconfig: invalid configuration")

	// ErrCatalogViaUpsert rejects dense_source="catalog" on the generic
	// upsert when the catalog strategy fields are absent → 422. Catalog mode
	// requires a strategy whose Dim matches embedding_dim; flipping the flag
	// without that validation wedged the namespace (the embedder
	// dead-lettered every item at strategy_resolve while BYOE writes started
	// returning 409). Supplying catalog_strategy_id + catalog_strategy_version
	// in the same request runs the full validation and is accepted.
	ErrCatalogViaUpsert = errors.New("nsconfig: dense_source=catalog requires catalog_strategy_id and catalog_strategy_version in the same request")

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
	codohuetypes.DenseSourceDisabled: true,
	codohuetypes.DenseSourceItem2Vec: true,
	codohuetypes.DenseSourceSVD:      true,
	codohuetypes.DenseSourceBYOE:     true,
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
		if *req.DenseSource == codohuetypes.DenseSourceCatalog {
			// The core mode is settable here since 006 — but only with its
			// strategy alongside, so the dim validation the dedicated catalog
			// endpoint performs can run in the same request.
			if req.CatalogStrategyID == nil || *req.CatalogStrategyID == "" ||
				req.CatalogStrategyVersion == nil || *req.CatalogStrategyVersion == "" {
				return ErrCatalogViaUpsert
			}
		} else if !upsertDenseSources[*req.DenseSource] {
			return fmt.Errorf("%w: dense_source must be one of disabled|item2vec|svd|byoe|catalog, got %q", ErrInvalidConfig, *req.DenseSource)
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
