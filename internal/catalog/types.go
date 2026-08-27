package catalog

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

type objectCursor struct {
	Version      int       `json:"v"`
	Namespace    string    `json:"ns"`
	ChangedSince string    `json:"since,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
	ID           int64     `json:"id"`
}

func encodeObjectCursor(cursor objectCursor) (string, error) {
	raw, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("encode catalog cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func decodeObjectCursor(raw, namespace, changedSince string) (*objectCursor, error) {
	if raw == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: malformed cursor", ErrInvalidRequest)
	}
	var cursor objectCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil || cursor.Version != 1 || cursor.ID < 1 || cursor.UpdatedAt.IsZero() {
		return nil, fmt.Errorf("%w: malformed cursor", ErrInvalidRequest)
	}
	if cursor.Namespace != namespace || cursor.ChangedSince != changedSince {
		return nil, fmt.Errorf("%w: cursor does not match namespace or changed_since", ErrInvalidRequest)
	}
	return &cursor, nil
}

// State enumerates the lifecycle states of a catalog item, matching the
// 'state' column on the catalog_items table (migration 010). Re-declared
// as typed constants here so handlers, services, and the embedder worker
// share a single source of truth.
type State string

// Lifecycle states for catalog_items rows. Values match the SQL CHECK
// constraint in migration 010 and the mirror in internal/embedder.
const (
	StatePending    State = "pending"
	StateInFlight   State = "in_flight"
	StateEmbedded   State = "embedded"
	StateFailed     State = "failed"
	StateDeadLetter State = "dead_letter"
)

// IngestRequest is the JSON body accepted by POST /v1/namespaces/{ns}/catalog.
//
// Re-exported from codohuetypes so external clients (e.g., the Go SDK) parse
// the same struct. Per Q4 of the spec clarifications, only the `content`
// field feeds the embedder and contributes to the content hash; the optional
// `metadata` field is stored verbatim and ignored by the embedder.
type IngestRequest = codohuetypes.CatalogIngestRequest

// BatchIngestRequest re-exports the batch wire type.
type BatchIngestRequest = codohuetypes.CatalogBatchIngestRequest

// Item is the in-memory representation of a row in catalog_items.
type Item struct {
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
	// dense_source is not "catalog" (handler maps to 404 to avoid leaking
	// namespace existence to unauthenticated probes).
	ErrNamespaceNotEnabled = errors.New("catalog: namespace not enabled for auto-embedding")

	// ErrNamespaceNotFound fires when no namespace_configs row exists for
	// the URL-supplied namespace (handler maps to 404, same body as above).
	ErrNamespaceNotFound = errors.New("catalog: namespace not found")
)
