package codohuetypes

// CatalogIngestRequest is the JSON body for POST /v1/namespaces/{ns}/catalog.
// Only the Content field feeds the embedder and contributes to the content
// hash; Metadata is stored verbatim alongside the row and ignored by the
// embedder. Namespace is intentionally absent — the URL path is the single
// source of truth.
type CatalogIngestRequest struct {
	ObjectID string         `json:"object_id"`
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata,omitempty"`
}
