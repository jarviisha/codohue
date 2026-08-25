package nslifecycle

import (
	"context"
	"errors"
	"time"
)

// NamespaceState is the durable mutation gate for one namespace name.
type NamespaceState string

// Namespace lifecycle states. Deleting is durable on purpose: a delete that
// dies halfway must not leave the namespace writable on restart.
const (
	StateActive   NamespaceState = "active"
	StateDeleting NamespaceState = "deleting"
	StateDeleted  NamespaceState = "deleted"
)

// SystemState is the durable application-wide mutation gate.
type SystemState string

// Application-wide lifecycle states; resetting blocks every namespace writer.
const (
	SystemActive    SystemState = "active"
	SystemResetting SystemState = "resetting"
)

// Lifecycle coordination errors. Callers distinguish them because they map to
// different HTTP statuses: not-found is 404, not-active and resetting are 409.
var (
	ErrNamespaceNotFound   = errors.New("namespace lifecycle not found")
	ErrNamespaceNotActive  = errors.New("namespace lifecycle is not active")
	ErrSystemResetting     = errors.New("system lifecycle is resetting")
	ErrLeaseRequired       = errors.New("matching namespace lifecycle lease is required")
	ErrLegacyEnvelopesOpen = errors.New("legacy envelopes are still enabled")
	ErrAdoptionEvidence    = errors.New("producer adoption evidence is required")
)

// NamespaceLifecycle is the durable tombstone for a namespace name.
type NamespaceLifecycle struct {
	Namespace             string
	Generation            int64
	State                 NamespaceState
	ActivatedAt           time.Time
	LegacyMessagesAllowed bool
	LastError             string
	UpdatedAt             time.Time
}

// SystemLifecycle is the singleton durable reset and rollout gate.
type SystemLifecycle struct {
	State                     SystemState
	LegacyEnvelopesDisabledAt *time.Time
	LegacyAdoptionEvidence    string
	LastError                 string
	UpdatedAt                 time.Time
}

// EnvelopeDisposition describes whether queued work may be processed.
type EnvelopeDisposition uint8

// Envelope dispositions. Stale work is ACKed and dropped, never retried — the
// generation it targets is gone, so no retry can make it valid.
const (
	EnvelopeProcess EnvelopeDisposition = iota + 1
	EnvelopeStale
)

// CleanupCandidate identifies one physical namespace generation that is no
// longer current and can be removed after the global legacy gate closes.
type CleanupCandidate struct {
	Namespace  string
	Generation int64
}

// Store is the durable surface used by lifecycle coordination.
type Store interface {
	Activate(ctx context.Context, namespace string) (*NamespaceLifecycle, error)
	GetNamespace(ctx context.Context, namespace string) (*NamespaceLifecycle, error)
	StartDelete(ctx context.Context, namespace string) (*NamespaceLifecycle, error)
	CompleteDelete(ctx context.Context, namespace string, generation int64) error
	RecordNamespaceError(ctx context.Context, namespace string, generation int64, message string) error
	GetSystem(ctx context.Context) (*SystemLifecycle, error)
	StartReset(ctx context.Context) error
	CompleteReset(ctx context.Context) error
	RecordResetError(ctx context.Context, message string) error
	DisableLegacy(ctx context.Context, adoptionEvidence string, at time.Time) (bool, error)
}
