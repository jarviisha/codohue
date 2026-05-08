package ingest

import (
	"time"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// Action is re-exported from codohuetypes so the wire type has a single
// source of truth shared with external clients (e.g., the Go SDK).
type Action = codohuetypes.Action

// Supported action types (re-exported from codohuetypes).
const (
	ActionView    = codohuetypes.ActionView
	ActionLike    = codohuetypes.ActionLike
	ActionComment = codohuetypes.ActionComment
	ActionShare   = codohuetypes.ActionShare
	ActionSkip    = codohuetypes.ActionSkip
)

// DefaultActionWeights defines the default weight for each action type.
var DefaultActionWeights = map[Action]float64{
	ActionView:    1,
	ActionLike:    5,
	ActionComment: 8,
	ActionShare:   10,
	ActionSkip:    -2,
}

// EventPayload is the event structure received from clients via Redis Streams
// or the HTTP ingest endpoint.
type EventPayload = codohuetypes.EventPayload

// HTTPIngestRequest is the request body for POST /v1/namespaces/{ns}/events.
// Namespace is intentionally absent because the path parameter is the single
// source of truth for HTTP ingest.
type HTTPIngestRequest struct {
	SubjectID       string            `json:"subject_id"`
	ObjectID        string            `json:"object_id"`
	Action          Action            `json:"action"`
	OccurredAt      time.Time         `json:"occurred_at"`
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
