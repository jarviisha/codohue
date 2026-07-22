package codohuetypes

// ObjectUpsertRequest is the JSON body for PUT /v1/namespaces/{ns}/objects/{id}.
//
// Per-object metadata that is independent of embedding, so it applies under
// every dense_source. Namespace and object id come from the URL path and are
// intentionally absent from the body.
type ObjectUpsertRequest struct {
	// AuthorSubjectID records which subject created this object. It shares
	// the id space of Event.SubjectID but is ownership metadata, not a
	// behavioural link: the subject↔object interaction graph lives only in
	// events. An empty value clears the attribution.
	AuthorSubjectID string `json:"author_subject_id"`
}

// ObjectResponse is returned by PUT /v1/namespaces/{ns}/objects/{id}.
type ObjectResponse struct {
	Namespace       string `json:"namespace"`
	ObjectID        string `json:"object_id"`
	AuthorSubjectID string `json:"author_subject_id,omitempty"`
	UpdatedAt       string `json:"updated_at"`
}
