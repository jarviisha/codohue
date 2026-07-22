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
