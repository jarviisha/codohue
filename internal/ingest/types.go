package ingest

import "time"

// Action represents a user behavior type.
type Action string

// Supported action types.
const (
	ActionView    Action = "VIEW"
	ActionLike    Action = "LIKE"
	ActionComment Action = "COMMENT"
	ActionShare   Action = "SHARE"
	ActionSkip    Action = "SKIP"
)

// DefaultActionWeights defines the default weight for each action type.
var DefaultActionWeights = map[Action]float64{
	ActionView:    1,
	ActionLike:    5,
	ActionComment: 8,
	ActionShare:   10,
	ActionSkip:    -2,
}

// EventPayload is the event structure received from Redis Streams (sent by the Main Backend).
type EventPayload struct {
	Namespace       string            `json:"namespace"`
	SubjectID       string            `json:"subject_id"`
	ObjectID        string            `json:"object_id"`
	Action          Action            `json:"action"`
	Timestamp       time.Time         `json:"timestamp"`
	ObjectCreatedAt *time.Time        `json:"object_created_at,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// Event is the record stored in the database after validation and weight assignment.
type Event struct {
	ID              int64
	Namespace       string
	SubjectID       string
	ObjectID        string
	Action          Action
	Weight          float64
	OccurredAt      time.Time
	ObjectCreatedAt *time.Time
}
