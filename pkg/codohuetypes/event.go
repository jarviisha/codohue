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
// A metadata field used to sit here, accepted on the wire and silently
// discarded — the events table has no column for it. It was removed
// deliberately (a contract must not advertise a capability the server does
// not have): the HTTP path now rejects it via DecodeStrict, which is the
// honest failure; the Redis path ignores unknown fields, so old producers
// keep working there.
type EventPayload struct {
	Namespace       string     `json:"namespace"`
	SubjectID       string     `json:"subject_id"`
	ObjectID        string     `json:"object_id"`
	Action          Action     `json:"action"`
	OccurredAt      time.Time  `json:"occurred_at"`
	ObjectCreatedAt *time.Time `json:"object_created_at,omitempty"`
}
