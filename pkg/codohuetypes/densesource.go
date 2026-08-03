package codohuetypes

// DenseSource names the single producer of a namespace's object dense
// vectors. It is namespace configuration, not a marshaled wire type, so it
// has no golden snapshot — the values are exported so clients and the server
// share one definition instead of hand-copied string literals.
type DenseSource = string

const (
	// DenseSourceDisabled turns dense retrieval off; only sparse CF serves.
	DenseSourceDisabled DenseSource = "disabled"
	// DenseSourceItem2Vec trains item vectors from co-interaction sequences
	// during the cron batch run.
	DenseSourceItem2Vec DenseSource = "item2vec"
	// DenseSourceSVD factorizes the interaction matrix during the cron batch run.
	DenseSourceSVD DenseSource = "svd"
	// DenseSourceBYOE leaves object dense vectors to the client via
	// PUT /v1/namespaces/{ns}/objects/{id}/embedding.
	DenseSourceBYOE DenseSource = "byoe"
	// DenseSourceCatalog derives object dense vectors from raw catalog
	// content via the embedder worker; this is the system's core mode.
	DenseSourceCatalog DenseSource = "catalog"
)
