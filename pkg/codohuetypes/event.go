package codohuetypes

import "time"

// Action represents a user behavior type sent to the ingest pipeline.
type Action string

// Supported action types.
const (
	ActionView    Action = "VIEW"
	ActionLike    Action = "LIKE"
	ActionComment Action = "COMMENT"
	ActionShare   Action = "SHARE"
	ActionSkip    Action = "SKIP"
)

// EventPayload is the behavioral event sent by clients to the ingest pipeline,
// either via Redis Streams or the HTTP ingest endpoint. The same wire shape is
// used on both transports — the namespace field is ignored on the HTTP path
// (the URL is authoritative there) and carried in the body on the Redis path.
type EventPayload struct {
	Namespace       string            `json:"namespace"`
	SubjectID       string            `json:"subject_id"`
	ObjectID        string            `json:"object_id"`
	Action          Action            `json:"action"`
	OccurredAt      time.Time         `json:"occurred_at"`
	ObjectCreatedAt *time.Time        `json:"object_created_at,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}
