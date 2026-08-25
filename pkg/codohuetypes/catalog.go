package codohuetypes

// CatalogIngestRequest is the JSON body for POST /v1/namespaces/{ns}/catalog.
// Only the Content field feeds the embedder and contributes to the content
// hash; Metadata is stored verbatim alongside the row and ignored by the
// embedder. Namespace is intentionally absent — the URL path is the single
// source of truth.
type CatalogIngestRequest struct {
	ObjectID string `json:"object_id"`
	Content  string `json:"content"`

	// AuthorSubjectID optionally records which subject created this object.
	// It shares the id space of Event.SubjectID but is pure ownership
	// metadata: it does NOT make the object "belong to" that subject in any
	// behavioural sense, and nothing in the recommendation path reads it.
	// The subject↔object interaction graph lives only in the events table.
	AuthorSubjectID string `json:"author_subject_id,omitempty"`

	Metadata map[string]any `json:"metadata,omitempty"`
}

// CatalogStreamItem is the payload published to CatalogStreamName — the
// durable fire-and-forget alternative to POST /v1/namespaces/{ns}/catalog.
// Unlike the HTTP body it carries Namespace, because a stream entry has no
// URL path. Validation is identical to the HTTP path once consumed.
type CatalogStreamItem struct {
	Namespace           string         `json:"namespace"`
	NamespaceGeneration int64          `json:"namespace_generation,omitempty"`
	ObjectID            string         `json:"object_id"`
	Content             string         `json:"content"`
	AuthorSubjectID     string         `json:"author_subject_id,omitempty"`
	Metadata            map[string]any `json:"metadata,omitempty"`
}

// CatalogBatchIngestRequest is the JSON body for
// POST /v1/namespaces/{ns}/catalog/batch. At most CatalogBatchMaxItems items
// per request; each item is validated independently, so one bad item does not
// fail the batch.
type CatalogBatchIngestRequest struct {
	Items []CatalogIngestRequest `json:"items"`
}

// CatalogBatchMaxItems caps a batch ingest request. 100 items bounds the
// request body at 100 × the per-item content cap.
const CatalogBatchMaxItems = 100

// CatalogBatchItemResult reports the per-item outcome of a batch ingest.
// Error is the machine-readable code of the single-item endpoint
// (e.g. "empty_content", "content_too_large") and empty on acceptance.
type CatalogBatchItemResult struct {
	ObjectID string `json:"object_id"`
	Accepted bool   `json:"accepted"`
	Error    string `json:"error,omitempty"`
}

// CatalogBatchIngestResponse is returned by the batch ingest endpoint with
// one result per submitted item, in request order.
type CatalogBatchIngestResponse struct {
	Namespace string                   `json:"namespace"`
	Accepted  int                      `json:"accepted"`
	Rejected  int                      `json:"rejected"`
	Results   []CatalogBatchItemResult `json:"results"`
}

// CatalogObjectSummary is one row of the reconciliation read: enough for a
// repair pass to decide whether an object needs re-sending, nothing more.
// UpdatedAt is RFC3339.
type CatalogObjectSummary struct {
	ObjectID  string `json:"object_id"`
	UpdatedAt string `json:"updated_at"`
}

// CatalogObjectsResponse is returned by
// GET /v1/namespaces/{ns}/catalog/objects (?changed_since=&limit=&offset=) —
// the data-plane answer to "which objects do you already hold". Ordered by
// updated_at ascending so a repair pass can page forward and resume from the
// last UpdatedAt it saw.
type CatalogObjectsResponse struct {
	Namespace  string                 `json:"namespace"`
	Items      []CatalogObjectSummary `json:"items"`
	Total      int                    `json:"total"`
	Limit      int                    `json:"limit"`
	Offset     int                    `json:"offset"`
	NextCursor string                 `json:"next_cursor,omitempty"`
}
