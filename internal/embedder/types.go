package embedder

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// State enumerates the lifecycle states of a catalog item, matching the
// 'state' column on the catalog_items table (migration 010). Mirrored here
// (rather than imported from internal/catalog) because the constitution
// forbids cross-domain imports between internal/catalog and internal/embedder.
type State string

const (
	StatePending    State = "pending"
	StateInFlight   State = "in_flight"
	StateEmbedded   State = "embedded"
	StateFailed     State = "failed"
	StateDeadLetter State = "dead_letter"
)

// PendingItem is the projection of catalog_items that the embedder service
// needs to process a single entry. content is loaded fresh from Postgres
// each time (the stream entry only carries the row id) so a re-ingest
// before processing is automatically picked up.
type PendingItem struct {
	ID              int64
	Namespace       string
	ObjectID        string
	Content         string
	ContentHash     []byte
	StrategyID      string // strategy id LAST USED to embed this row, may be empty for never-embedded
	StrategyVersion string // strategy version LAST USED, may be empty for never-embedded
	AttemptCount    int
}

// StreamEntry is the decoded form of an XMessage from catalog:embed:{ns}.
// Per contracts/redis-stream.md, only catalog_item_id is authoritative —
// the worker re-reads everything else from Postgres. The remaining fields
// are kept for log greppability and audit only.
type StreamEntry struct {
	EntryID       string // Redis Streams entry id, e.g. "1700000000-0"
	CatalogItemID int64
	Namespace     string
	ObjectID      string
	StrategyID    string
	StrategyVer   string
	EnqueuedAt    time.Time
}

// DecodeStreamEntry converts a Redis XMessage into a StreamEntry, validating
// that all required fields are present and well-typed. Fields that are
// merely informational (object_id, strategy_*, enqueued_at) tolerate empty
// or unparseable values — they are NOT used as authority.
func DecodeStreamEntry(msg redis.XMessage) (*StreamEntry, error) {
	itemIDRaw, ok := msg.Values["catalog_item_id"]
	if !ok {
		return nil, fmt.Errorf("stream entry %s: missing catalog_item_id", msg.ID)
	}
	itemIDStr, ok := itemIDRaw.(string)
	if !ok {
		return nil, fmt.Errorf("stream entry %s: catalog_item_id is %T, want string", msg.ID, itemIDRaw)
	}
	itemID, err := strconv.ParseInt(itemIDStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("stream entry %s: parse catalog_item_id: %w", msg.ID, err)
	}

	entry := &StreamEntry{
		EntryID:       msg.ID,
		CatalogItemID: itemID,
		Namespace:     stringField(msg.Values, "namespace"),
		ObjectID:      stringField(msg.Values, "object_id"),
		StrategyID:    stringField(msg.Values, "strategy_id"),
		StrategyVer:   stringField(msg.Values, "strategy_version"),
	}
	if raw := stringField(msg.Values, "enqueued_at"); raw != "" {
		if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			entry.EnqueuedAt = t
		}
	}
	return entry, nil
}

func stringField(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// Embedder service-level errors. The worker maps these to catalog_items
// state transitions in the FR-010 retry / dead-letter contract: hard
// errors (e.g. zero-norm) → dead_letter immediately; transient errors
// → failed with attempt_count++, retried until max_attempts.
var (
	// ErrItemNotFound is returned by Repository.LoadByID when no row matches.
	// The worker ACKs the stream entry and skips — likely a race with delete.
	ErrItemNotFound = errors.New("embedder: catalog item not found")

	// ErrNamespaceNotEnabled is returned when the embedder picks up an
	// entry for a namespace that is no longer catalog-enabled. The worker
	// ACKs the entry and skips.
	ErrNamespaceNotEnabled = errors.New("embedder: namespace not enabled")
)
