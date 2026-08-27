package idmap

import (
	"errors"
	"time"
)

// RepairRunState is the durable phase of one migration-022 reconciliation.
//
// The run is a state machine rather than one transaction because it spans two
// stores: a PostgreSQL mapping change and a Qdrant point copy cannot commit
// together, so a crash must be resumable from durable state instead of rolled
// back.
type RepairRunState string

// Run phases in order. `failed` is resumable — it records where the run
// stopped, it does not undo what already landed.
const (
	RepairRunAudited      RepairRunState = "audited"
	RepairRunSnapshotting RepairRunState = "snapshotting"
	RepairRunApplying     RepairRunState = "applying"
	RepairRunVerifying    RepairRunState = "verifying"
	RepairRunComplete     RepairRunState = "complete"
	RepairRunFailed       RepairRunState = "failed"
)

// RepairItemState is the per-identity progress within a run.
type RepairItemState string

// Item states. `quarantined` is terminal for the item and blocking for the
// run: it means the audit could not decide which numeric id is authoritative,
// and guessing is exactly what must not happen.
const (
	RepairItemPending     RepairItemState = "pending"
	RepairItemCopied      RepairItemState = "copied"
	RepairItemVerified    RepairItemState = "verified"
	RepairItemCleaned     RepairItemState = "cleaned"
	RepairItemQuarantined RepairItemState = "quarantined"
	RepairItemFailed      RepairItemState = "failed"
)

// Repair workflow errors.
var (
	// ErrRepairRunNotFound is returned when a run id does not exist.
	ErrRepairRunNotFound = errors.New("idmap: repair run not found")
	// ErrRepairRunActive is returned when a second run is requested while one
	// is still in flight. Two reconciliations would each believe they own the
	// numeric-id space.
	ErrRepairRunActive = errors.New("idmap: a repair run is already in progress")
	// ErrManifestChanged is returned when the audited item set no longer
	// hashes to what the run recorded. The operator reviewed a specific set of
	// decisions; applying a different one silently is the failure this whole
	// workflow exists to prevent.
	ErrManifestChanged = errors.New("idmap: repair manifest changed since audit")
	// ErrQuarantinedItems is returned when apply is attempted with unresolved
	// evidence. Zero points and zero mappings are mutated.
	ErrQuarantinedItems = errors.New("idmap: repair manifest contains quarantined items")
	// ErrSnapshotsRequired is returned when apply is attempted without a
	// recorded PostgreSQL backup and a snapshot for every affected collection.
	// Without them a partial apply has no recovery path.
	ErrSnapshotsRequired = errors.New("idmap: coordinated snapshots are required before apply")
)

// RepairRun is one durable reconciliation.
type RepairRun struct {
	ID                 int64
	State              RepairRunState
	PGSnapshotRef      string
	QdrantSnapshotRefs map[string]string
	ManifestHash       string
	// RebuiltNamespaces records which namespaces had their sparse vectors
	// rebuilt. Verification asserts coverage against the manifest rather than
	// inferring it from the run having reached the verifying phase.
	RebuiltNamespaces []string
	StartedAt         time.Time
	CompletedAt       *time.Time
	Error             string
}

// RepairItem is one audited logical identity within a run.
//
// OldNumericIDs is a slice because the audit's whole purpose is finding
// identities that resolve to more than one numeric id across stores.
type RepairItem struct {
	RunID           int64
	Namespace       string
	EntityType      string
	StringID        string
	OldNumericIDs   []int64
	TargetNumericID *int64
	Sources         map[string]any
	PayloadHash     string
	VectorHash      string
	State           RepairItemState
	Error           string
}

// sourceTargetConflict is the Sources key recording that the mapped numeric id
// is already held by a different identity in one of this item's collections.
const sourceTargetConflict = "target_conflict_with"

// sourceTargetReassignedFrom records the numeric id the audit wanted to use
// before reassigning this item to a fresh one.
const sourceTargetReassignedFrom = "target_reassigned_from"

// Resolved reports whether the audit selected an authoritative numeric id for
// this identity. An unresolved item blocks apply for the whole run.
func (i RepairItem) Resolved() bool {
	return i.State != RepairItemQuarantined && i.TargetNumericID != nil
}

// TargetConflictWith returns the string id of the identity already occupying
// this item's mapped numeric id, or "" when the target is free.
//
// The occupancy matters because a copy onto an occupied id destroys whatever
// was there, and the copy's own verification cannot detect it — it compares
// the destination against the source it just wrote, not against what the
// destination held before.
func (i RepairItem) TargetConflictWith() string {
	if i.Sources == nil {
		return ""
	}
	// A manifest written before this key existed, or one carrying a
	// non-string value, reads as "no conflict" rather than failing the run.
	conflict, ok := i.Sources[sourceTargetConflict].(string)
	if !ok {
		return ""
	}
	return conflict
}

// NeedsCopy reports whether the item's dense point still has to be moved. An
// identity whose only numeric id is already the target needs no point copy —
// preserving a correct mapping is cheaper and safer than rewriting it.
func (i RepairItem) NeedsCopy() bool {
	if !i.Resolved() {
		return false
	}
	for _, old := range i.OldNumericIDs {
		if old != *i.TargetNumericID {
			return true
		}
	}
	return false
}
