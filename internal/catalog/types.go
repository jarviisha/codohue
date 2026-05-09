package catalog

import (
	"crypto/sha256"
	"errors"
	"time"
)

// State enumerates the lifecycle states of a catalog item, matching the
// 'state' column on the catalog_items table (migration 010). Re-declared
// as typed constants here so handlers, services, and the embedder worker
// share a single source of truth.
type State string

const (
	StatePending    State = "pending"
	StateInFlight   State = "in_flight"
	StateEmbedded   State = "embedded"
	StateFailed     State = "failed"
	StateDeadLetter State = "dead_letter"
)

// IngestRequest is the JSON body accepted by POST /v1/namespaces/{ns}/catalog.
//
// Per Q4 of the spec clarifications, only the `content` field feeds the
// embedder and contributes to the content hash. The optional `metadata`
// field is stored verbatim alongside the row and ignored by the embedder.
//
// `namespace` is intentionally absent from the body — the URL path is the
// single source of truth (consistent with the 003 RESTful redesign).
type IngestRequest struct {
	ObjectID string         `json:"object_id"`
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// CatalogItem is the in-memory representation of a row in catalog_items.
type CatalogItem struct {
	ID              int64
	Namespace       string
	ObjectID        string
	Content         string
	ContentHash     []byte
	Metadata        map[string]any
	State           State
	StrategyID      string
	StrategyVersion string
	EmbeddedAt      *time.Time
	AttemptCount    int
	LastError       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ContentHash returns the canonical sha256 of the embedding input. Only
// `content` is hashed; metadata is excluded by FR-002 / Q4 so that
// metadata-only re-ingestion is idempotent at the embedding layer.
func ContentHash(content string) []byte {
	sum := sha256.Sum256([]byte(content))
	return sum[:]
}

// Sentinel errors so callers (handler, service) and tests can branch on
// failure mode without parsing strings.
var (
	// ErrInvalidRequest covers shape problems the handler should map to 400.
	ErrInvalidRequest = errors.New("catalog: invalid request")

	// ErrEmptyContent fires when content trims to empty (handler maps to 422).
	ErrEmptyContent = errors.New("catalog: content is empty after trimming")

	// ErrContentTooLarge fires when len(content) exceeds the namespace's
	// catalog_max_content_bytes cap (handler maps to 413).
	ErrContentTooLarge = errors.New("catalog: content exceeds catalog_max_content_bytes")

	// ErrNamespaceNotEnabled fires when the namespace exists but its
	// catalog_enabled is false (handler maps to 404 to avoid leaking
	// namespace existence to unauthenticated probes).
	ErrNamespaceNotEnabled = errors.New("catalog: namespace not enabled for auto-embedding")

	// ErrNamespaceNotFound fires when no namespace_configs row exists for
	// the URL-supplied namespace (handler maps to 404, same body as above).
	ErrNamespaceNotFound = errors.New("catalog: namespace not found")
)
