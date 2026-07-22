package objects

import (
	"errors"
	"time"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

// UpsertRequest is the JSON body accepted by
// PUT /v1/namespaces/{ns}/objects/{id}.
//
// Re-exported from codohuetypes so external clients parse the same struct.
type UpsertRequest = codohuetypes.ObjectUpsertRequest

// Object is the in-memory representation of a row in the objects table.
type Object struct {
	Namespace       string
	ObjectID        string
	AuthorSubjectID string // empty when unattributed
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ErrInvalidRequest covers shape problems the handler maps to 400.
var ErrInvalidRequest = errors.New("objects: invalid request")
