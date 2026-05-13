package embedder

import "github.com/jarviisha/codohue/internal/core/embedstrategy"

// init registers the V1 hashing+ngrams strategy with the default registry.
// Importing this package anywhere in the binary is sufficient to make the
// V1 strategy resolvable from embedstrategy.DefaultRegistry().Build —
// cmd/admin imports it for namespace-config validation, cmd/embedder for
// per-item embedding, and cmd/api never touches it directly (catalog
// ingest only reads namespace state, never builds a Strategy).
//
// Adding a future external-LLM strategy means dropping a new file under
// internal/embedder/ with its own init() that calls Register or
// RegisterVariants — no other code needs to change. That is the FR-021
// forward-compat seam working as designed.
func init() {
	variants := []embedstrategy.StrategyDescriptor{
		{
			ID: hashingNgramsID, Version: hashingNgramsVersion, Dim: 64,
			Description: "Pure-Go feature hashing + character n-grams (n=3..5), 64-d",
		},
		{
			ID: hashingNgramsID, Version: hashingNgramsVersion, Dim: 128,
			Description: "Pure-Go feature hashing + character n-grams (n=3..5), 128-d",
		},
		{
			ID: hashingNgramsID, Version: hashingNgramsVersion, Dim: 256,
			Description: "Pure-Go feature hashing + character n-grams (n=3..5), 256-d",
		},
		{
			ID: hashingNgramsID, Version: hashingNgramsVersion, Dim: 512,
			Description: "Pure-Go feature hashing + character n-grams (n=3..5), 512-d",
		},
	}
	embedstrategy.DefaultRegistry().RegisterVariants(
		hashingNgramsID, hashingNgramsVersion, newHashingNgrams, variants,
	)
}
