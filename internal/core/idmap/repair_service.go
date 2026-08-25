package idmap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
)

// CollectionEvidence is one point as the audit sees it, flattened so this
// package needs no Qdrant dependency. The composition layer supplies it.
type CollectionEvidence struct {
	Collection  string
	EntityType  string
	NumericID   int64
	StringID    string
	PayloadHash string
	VectorHash  string
}

// NamespaceEvidence is everything the audit knows about one namespace: the
// PostgreSQL mappings and every point across its collections.
type NamespaceEvidence struct {
	Namespace string
	// Mappings is entity_type → string_id → numeric_id.
	Mappings map[string]map[string]int64
	Points   []CollectionEvidence
	// DenseCollections are the collections holding unrecomputable vectors, so
	// the repair knows which points must be copied rather than rebuilt.
	DenseCollections map[string]bool
}

// EvidenceSource supplies the audit's raw material.
type EvidenceSource interface {
	Namespaces(ctx context.Context) ([]string, error)
	Evidence(ctx context.Context, namespace string) (*NamespaceEvidence, error)
}

// InspectedPoint is what verification reads back from a repaired location.
//
// The two hashes are the preservation checks the audit recorded: the payload
// carries `created_at`, which drives the γ-freshness rerank, and the vector is
// the thing that may not be recomputable at all.
type InspectedPoint struct {
	StringID    string
	PayloadHash string
	VectorHash  string
}

// PointMover copies and removes dense points. Implemented over
// internal/infra/qdrant by the composition layer.
type PointMover interface {
	CopyPointVerified(ctx context.Context, collection string, from, to int64) error
	DeletePoint(ctx context.Context, collection string, id int64) error
	PointAbsent(ctx context.Context, collection string, id int64) (bool, error)
	// InspectPoint returns what is stored at a numeric id. Verification needs
	// it to prove the target holds the right point rather than trusting that
	// apply's own check ran — a resumed run skips items already marked
	// cleaned, so nothing else would confirm them.
	InspectPoint(ctx context.Context, collection string, id int64) (point InspectedPoint, found bool, err error)
}

// SparseRebuilder is the narrow port through which the repair triggers a full
// sparse recompute.
//
// It exists so this package never imports internal/compute: sparse object
// vectors encode subject numeric ids in their coordinates, so they must be
// rebuilt after mappings settle — but compute already depends on idmap, and
// importing it back would make that cycle real. cmd/admin adapts compute to
// this interface instead.
type SparseRebuilder interface {
	RebuildSparse(ctx context.Context, namespace string) error
}

// GlobalFence runs a function with every writer in the fleet frozen.
type GlobalFence interface {
	WithGlobalExclusive(ctx context.Context, fn func(context.Context) error) error
}

// repairStore is the durable surface the orchestration needs. Declared as an
// interface so the state machine can be driven without PostgreSQL;
// *RepairRepository satisfies it.
type repairStore interface {
	ActiveRun(ctx context.Context) (*RepairRun, error)
	CreateRun(ctx context.Context, items []RepairItem, startedAt time.Time) (*RepairRun, error)
	GetRun(ctx context.Context, id int64) (*RepairRun, error)
	ListItems(ctx context.Context, runID int64) ([]RepairItem, error)
	ListItemsInState(ctx context.Context, runID int64, states ...RepairItemState) ([]RepairItem, error)
	NextNumericID(ctx context.Context) (int64, error)
	RecordRebuiltNamespace(ctx context.Context, id int64, namespace string) error
	RecordSnapshots(ctx context.Context, id int64, pgRef string, qdrantRefs map[string]string) error
	RetargetMapping(ctx context.Context, namespace, entityType, stringID string, numericID int64) error
	SetItemState(ctx context.Context, item RepairItem, state RepairItemState, errMessage string) error
	SetRunState(ctx context.Context, id int64, state RepairRunState, errMessage string) error
}

// RepairService orchestrates the migration-022 reconciliation.
type RepairService struct {
	repo     repairStore
	evidence EvidenceSource
	mover    PointMover
	rebuild  SparseRebuilder
	fence    GlobalFence
	now      func() time.Time
}

// NewRepairService wires the reconciliation. rebuild and fence may be nil in a
// read-only audit deployment; apply requires both.
func NewRepairService(repo *RepairRepository, evidence EvidenceSource, mover PointMover, rebuild SparseRebuilder, fence GlobalFence) *RepairService {
	return &RepairService{repo: repo, evidence: evidence, mover: mover, rebuild: rebuild, fence: fence, now: time.Now}
}

// AuditReport is the read-only outcome of an audit.
type AuditReport struct {
	RunID        int64
	ManifestHash string
	Total        int
	Resolved     int
	NeedsRepair  int
	Quarantined  []RepairItem
}

// Audit inventories both stores and records an immutable manifest. It mutates
// no point and no mapping — the operator reviews the decisions before anything
// moves.
func (s *RepairService) Audit(ctx context.Context) (*AuditReport, error) {
	active, err := s.repo.ActiveRun(ctx)
	if err != nil {
		return nil, err
	}
	if active != nil {
		if !supersedable(active.State) {
			return nil, fmt.Errorf("%w: run %d is in state %s", ErrRepairRunActive, active.ID, active.State)
		}
		// Resolving a quarantined item changes the evidence, so the operator
		// re-audits. The previous run has moved nothing, and an `audited` run
		// never reaches a terminal state on its own — without this it would
		// block every later audit forever.
		if err := s.repo.SetRunState(ctx, active.ID, RepairRunFailed, "superseded by a later audit"); err != nil {
			return nil, fmt.Errorf("supersede run %d: %w", active.ID, err)
		}
		slog.InfoContext(ctx, "idmap repair: superseded a pre-mutation run", "run_id", active.ID, "state", active.State)
	}

	namespaces, err := s.evidence.Namespaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("list namespaces for audit: %w", err)
	}

	var items []RepairItem
	for _, namespace := range namespaces {
		evidence, err := s.evidence.Evidence(ctx, namespace)
		if err != nil {
			return nil, fmt.Errorf("collect evidence for %q: %w", namespace, err)
		}
		items = append(items, auditNamespace(evidence)...)
	}

	// Resolve contested targets before the manifest is frozen, so what the
	// operator reviews is the id the repair will actually write to.
	if err := reassignContestedTargets(ctx, s.repo, items); err != nil {
		return nil, err
	}

	run, err := s.repo.CreateRun(ctx, items, s.now().UTC())
	if err != nil {
		return nil, err
	}

	report := &AuditReport{RunID: run.ID, ManifestHash: run.ManifestHash, Total: len(items)}
	for _, item := range items {
		switch {
		case item.State == RepairItemQuarantined:
			report.Quarantined = append(report.Quarantined, item)
		case item.NeedsCopy():
			report.Resolved++
			report.NeedsRepair++
		default:
			report.Resolved++
		}
	}
	return report, nil
}

// numericIDReserver mints numeric ids from the same sequence the hot path uses.
// Declared as an interface so the reassignment rule can be tested without a
// database.
type numericIDReserver interface {
	NextNumericID(ctx context.Context) (int64, error)
}

// reassignContestedTargets moves every identity whose mapped numeric id is held
// by a different identity onto a freshly reserved id.
//
// The alternative — evicting the occupant — is not available: the occupant's
// vector may be exactly as unrecomputable as this one's, so neither may be
// sacrificed for the other. Minting a third id resolves the conflict without
// touching either vector; apply then retargets the mapping to it, which is the
// one case where the mapping genuinely changes.
//
// Reserving a sequence value is the only side effect an audit has beyond
// writing its own manifest: it mutates no point and no mapping.
func reassignContestedTargets(ctx context.Context, reserver numericIDReserver, items []RepairItem) error {
	for i := range items {
		item := &items[i]
		if !item.Resolved() || item.TargetConflictWith() == "" {
			continue
		}
		fresh, err := reserver.NextNumericID(ctx)
		if err != nil {
			return fmt.Errorf("reserve a fresh numeric id for %s/%s/%s: %w",
				item.Namespace, item.EntityType, item.StringID, err)
		}
		item.Sources[sourceTargetReassignedFrom] = *item.TargetNumericID
		item.TargetNumericID = &fresh
	}
	return nil
}

// verifyItem re-checks one repaired identity and returns every problem found.
//
// Plan Release 4 step 7 asks verification to confirm the manifest tuple, the
// payload id, the vector hash and old-point absence. Checking absence alone
// would pass a run whose replacement point never landed.
func verifyItem(ctx context.Context, mover PointMover, item RepairItem) []string {
	if !item.NeedsCopy() {
		// Nothing moved, so there is no new location to confirm.
		return nil
	}
	target := *item.TargetNumericID
	identity := fmt.Sprintf("%s/%s/%s", item.Namespace, item.EntityType, item.StringID)

	var problems []string
	for _, collection := range copyableCollections(item) {
		point, found, err := mover.InspectPoint(ctx, collection, target)
		switch {
		case err != nil:
			problems = append(problems, fmt.Sprintf("%s: could not read %s#%d: %v", identity, collection, target, err))
			continue
		case !found:
			problems = append(problems, fmt.Sprintf("%s: target point %s#%d is missing", identity, collection, target))
			continue
		case point.StringID != item.StringID:
			problems = append(problems, fmt.Sprintf("%s: target point %s#%d carries %q", identity, collection, target, point.StringID))
			continue
		}
		// A manifest written before a given hash was recorded cannot be checked
		// against it; every other check still runs.
		if item.VectorHash != "" && point.VectorHash != item.VectorHash {
			problems = append(problems, fmt.Sprintf("%s: vector at %s#%d changed", identity, collection, target))
		}
		if item.PayloadHash != "" && point.PayloadHash != item.PayloadHash {
			problems = append(problems, fmt.Sprintf("%s: payload at %s#%d changed", identity, collection, target))
		}

		for _, old := range item.OldNumericIDs {
			if old == target {
				continue
			}
			absent, err := mover.PointAbsent(ctx, collection, old)
			if err != nil {
				problems = append(problems, fmt.Sprintf("%s: could not check %s#%d: %v", identity, collection, old, err))
				continue
			}
			if !absent {
				problems = append(problems, fmt.Sprintf("%s: old point %s#%d still present", identity, collection, old))
			}
		}
	}
	return problems
}

// unrebuiltNamespaces lists the manifest's namespaces that have no rebuild
// record, sorted and deduplicated so the operator sees each one once.
func unrebuiltNamespaces(items []RepairItem, rebuilt []string) []string {
	done := make(map[string]struct{}, len(rebuilt))
	for _, namespace := range rebuilt {
		done[namespace] = struct{}{}
	}
	seen := map[string]struct{}{}
	var missing []string
	for _, item := range items {
		if _, already := seen[item.Namespace]; already {
			continue
		}
		seen[item.Namespace] = struct{}{}
		if _, ok := done[item.Namespace]; !ok {
			missing = append(missing, item.Namespace)
		}
	}
	sort.Strings(missing)
	return missing
}

// missingSnapshotCollections lists the collections this manifest touches that
// have no recorded snapshot, sorted so the operator sees a stable list.
//
// Every manifest row counts, including quarantined ones: coverage is about
// what apply could reach, not about which rows happen to be actionable today.
func missingSnapshotCollections(items []RepairItem, refs map[string]string) []string {
	needed := map[string]struct{}{}
	for _, item := range items {
		for _, collection := range itemCollections(item) {
			needed[collection] = struct{}{}
		}
	}
	var missing []string
	for collection := range needed {
		if strings.TrimSpace(refs[collection]) == "" {
			missing = append(missing, collection)
		}
	}
	sort.Strings(missing)
	return missing
}

// supersedable reports whether a re-audit may replace this run.
//
// Only runs that have mutated nothing qualify. Once apply has started, points
// and mappings may already have moved, so a fresh manifest would describe a
// half-repaired fleet — the operator has to resume or verify instead.
func supersedable(state RepairRunState) bool {
	return state != RepairRunApplying && state != RepairRunVerifying
}

// auditNamespace decides, for each logical identity, which numeric id is
// authoritative — or refuses to decide.
//
// The rule is deliberately narrow: the PostgreSQL mapping is authoritative
// when it exists, because that is what every live lookup already returns.
// Anything the mapping cannot explain — a point claiming an identity with no
// mapping, or two points claiming one identity in the same collection — is
// quarantined. Picking a winner there would silently merge or orphan data
// nobody asked us to touch.
func auditNamespace(evidence *NamespaceEvidence) []RepairItem {
	type observed struct {
		numericIDs  map[int64]struct{}
		collections map[string][]int64
		payloadHash string
		vectorHash  string
		unlabeled   bool
	}
	byIdentity := map[string]*observed{}
	// occupancy answers "which identity currently holds numeric id N in this
	// collection". A target id held by a DIFFERENT identity cannot be copied
	// onto without destroying that identity's vector.
	occupancy := map[string]map[int64]string{}
	key := func(entityType, stringID string) string { return entityType + "\x1f" + stringID }

	for _, point := range evidence.Points {
		if point.StringID == "" {
			// A point with no payload identity cannot be associated with a
			// mapping at all. Record it against a synthetic key so the run is
			// blocked rather than silently leaving it behind.
			k := key(point.EntityType, fmt.Sprintf("\x00unlabeled:%s:%d", point.Collection, point.NumericID))
			byIdentity[k] = &observed{
				numericIDs:  map[int64]struct{}{point.NumericID: {}},
				collections: map[string][]int64{point.Collection: {point.NumericID}},
				unlabeled:   true,
			}
			continue
		}
		k := key(point.EntityType, point.StringID)
		entry := byIdentity[k]
		if entry == nil {
			entry = &observed{numericIDs: map[int64]struct{}{}, collections: map[string][]int64{}}
			byIdentity[k] = entry
		}
		entry.numericIDs[point.NumericID] = struct{}{}
		entry.collections[point.Collection] = append(entry.collections[point.Collection], point.NumericID)
		if occupancy[point.Collection] == nil {
			occupancy[point.Collection] = map[int64]string{}
		}
		occupancy[point.Collection][point.NumericID] = k
		if point.PayloadHash != "" {
			entry.payloadHash = point.PayloadHash
		}
		if point.VectorHash != "" {
			entry.vectorHash = point.VectorHash
		}
	}

	// Mappings with no points still belong in the manifest: verification
	// checks the whole logical set, not only what happened to be stored.
	for entityType, mappings := range evidence.Mappings {
		for stringID := range mappings {
			k := key(entityType, stringID)
			if byIdentity[k] == nil {
				byIdentity[k] = &observed{numericIDs: map[int64]struct{}{}, collections: map[string][]int64{}}
			}
		}
	}

	items := make([]RepairItem, 0, len(byIdentity))
	for k, entry := range byIdentity {
		entityType, stringID, _ := splitKey(k)
		// dense_collections narrows what apply copies: sparse coordinates
		// encode subject numeric ids, so they are rebuilt by the recompute
		// rather than moved point by point.
		dense := map[string]any{}
		for collection := range entry.collections {
			if evidence.DenseCollections[collection] {
				dense[collection] = true
			}
		}
		item := RepairItem{
			Namespace:   evidence.Namespace,
			EntityType:  entityType,
			StringID:    stringID,
			PayloadHash: entry.payloadHash,
			VectorHash:  entry.vectorHash,
			State:       RepairItemPending,
			Sources: map[string]any{
				"collections":       entry.collections,
				"dense_collections": dense,
			},
		}
		for id := range entry.numericIDs {
			item.OldNumericIDs = append(item.OldNumericIDs, id)
		}
		sort.Slice(item.OldNumericIDs, func(i, j int) bool { return item.OldNumericIDs[i] < item.OldNumericIDs[j] })

		switch {
		case entry.unlabeled:
			item.State = RepairItemQuarantined
			item.Error = "point payload carries no logical id; cannot associate it with a mapping"
		case hasAmbiguousCollection(entry.collections):
			item.State = RepairItemQuarantined
			item.Error = "two points in one collection claim this identity; the authoritative vector is ambiguous"
		default:
			mapped, ok := evidence.Mappings[entityType][stringID]
			if !ok {
				item.State = RepairItemQuarantined
				item.Error = "points exist for an identity with no id_mappings row; no authoritative numeric id"
				break
			}
			target := mapped
			item.TargetNumericID = &target
			if holder := targetHolder(occupancy, entry.collections, target, k); holder != "" {
				_, conflictID, _ := splitKey(holder)
				item.Sources[sourceTargetConflict] = conflictID
			}
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].EntityType != items[j].EntityType {
			return items[i].EntityType < items[j].EntityType
		}
		return items[i].StringID < items[j].StringID
	})
	return items
}

// targetHolder reports the identity key occupying target in any collection this
// item appears in, or "" when the target is free or already this item's own.
//
// Occupancy is per collection: the same numeric id in a different collection is
// a different vector space, so it is not a conflict.
func targetHolder(occupancy map[string]map[int64]string, collections map[string][]int64, target int64, self string) string {
	for collection := range collections {
		holder, ok := occupancy[collection][target]
		if ok && holder != self {
			return holder
		}
	}
	return ""
}

func hasAmbiguousCollection(collections map[string][]int64) bool {
	for _, ids := range collections {
		distinct := map[int64]struct{}{}
		for _, id := range ids {
			distinct[id] = struct{}{}
		}
		if len(distinct) > 1 {
			return true
		}
	}
	return false
}

func splitKey(k string) (entityType, stringID string, ok bool) {
	for i := 0; i < len(k); i++ {
		if k[i] == '\x1f' {
			return k[:i], k[i+1:], true
		}
	}
	return k, "", false
}

// PrepareSnapshots records the coordinated backup references taken while the
// global lease is held. Apply refuses to run without them.
func (s *RepairService) PrepareSnapshots(ctx context.Context, runID int64, pgRef string, qdrantRefs map[string]string) error {
	run, err := s.repo.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if run.State != RepairRunAudited && run.State != RepairRunSnapshotting {
		return fmt.Errorf("run %d is in state %s; snapshots are recorded after audit", runID, run.State)
	}
	if err := s.repo.RecordSnapshots(ctx, runID, pgRef, qdrantRefs); err != nil {
		return err
	}
	return s.repo.SetRunState(ctx, runID, RepairRunSnapshotting, "")
}

// Apply moves every resolved identity onto its authoritative numeric id under
// the global exclusive lease.
//
// It refuses to start when the manifest changed since audit, when any item is
// quarantined, or when the coordinated snapshots are missing. Those are the
// three ways an apply could destroy data it cannot restore.
func (s *RepairService) Apply(ctx context.Context, runID int64) error {
	if s.fence == nil {
		return errors.New("idmap: apply requires the global lifecycle fence")
	}
	return s.fence.WithGlobalExclusive(ctx, func(fenced context.Context) error {
		return s.applyFenced(fenced, runID)
	})
}

func (s *RepairService) applyFenced(ctx context.Context, runID int64) error {
	run, err := s.repo.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	items, err := s.repo.ListItems(ctx, runID)
	if err != nil {
		return err
	}
	if got := ManifestHash(items); got != run.ManifestHash {
		return fmt.Errorf("%w: manifest hashes to %s, run recorded %s", ErrManifestChanged, got, run.ManifestHash)
	}
	if quarantined := filterState(items, RepairItemQuarantined); len(quarantined) > 0 {
		return fmt.Errorf("%w: %d unresolved item(s), first is %s/%s/%s (%s)",
			ErrQuarantinedItems, len(quarantined),
			quarantined[0].Namespace, quarantined[0].EntityType, quarantined[0].StringID, quarantined[0].Error)
	}
	if run.PGSnapshotRef == "" {
		return fmt.Errorf("%w: no PostgreSQL backup reference recorded", ErrSnapshotsRequired)
	}
	// Coverage is per collection, not "at least one". A run spanning four
	// collections that recorded a single snapshot would mutate three of them
	// with no way back, and would report success doing it.
	if missing := missingSnapshotCollections(items, run.QdrantSnapshotRefs); len(missing) > 0 {
		return fmt.Errorf("%w: no snapshot recorded for %s", ErrSnapshotsRequired, strings.Join(missing, ", "))
	}

	if err := s.repo.SetRunState(ctx, runID, RepairRunApplying, ""); err != nil {
		return err
	}

	namespaces := map[string]struct{}{}
	for _, item := range items {
		namespaces[item.Namespace] = struct{}{}
		if item.State == RepairItemCleaned || item.State == RepairItemVerified {
			continue // already applied by an earlier attempt; resume skips it
		}
		if err := s.applyItem(ctx, item); err != nil {
			// Recording where the run stopped is what makes it resumable, so
			// a failure to record is logged but must not mask the real error.
			s.recordFailure(ctx, runID, &item, err)
			return fmt.Errorf("apply %s/%s/%s: %w", item.Namespace, item.EntityType, item.StringID, err)
		}
	}

	// Sparse coordinates encode subject numeric ids, so they are rebuilt only
	// after every mapping has settled.
	if s.rebuild != nil {
		for _, namespace := range sortedNamespaces(namespaces) {
			if err := s.rebuild.RebuildSparse(ctx, namespace); err != nil {
				s.recordFailure(ctx, runID, nil, err)
				return fmt.Errorf("rebuild sparse vectors for %q: %w", namespace, err)
			}
			// Recorded per namespace as it completes, so an interrupted apply
			// leaves an accurate partial record for verification to read.
			if err := s.repo.RecordRebuiltNamespace(ctx, runID, namespace); err != nil {
				s.recordFailure(ctx, runID, nil, err)
				return err
			}
		}
	}

	return s.repo.SetRunState(ctx, runID, RepairRunVerifying, "")
}

// recordFailure persists where a run stopped. Both writes are best-effort:
// losing the note costs the operator a diagnostic, while masking the original
// error would cost them the reason.
func (s *RepairService) recordFailure(ctx context.Context, runID int64, item *RepairItem, cause error) {
	if item != nil {
		if err := s.repo.SetItemState(ctx, *item, RepairItemFailed, cause.Error()); err != nil {
			slog.WarnContext(ctx, "persist repair item failure state failed", "run_id", runID, "error", err)
		}
	}
	if err := s.repo.SetRunState(ctx, runID, RepairRunFailed, cause.Error()); err != nil {
		slog.WarnContext(ctx, "persist repair run failure state failed", "run_id", runID, "error", err)
	}
}

// applyItem moves one identity: copy the dense point to the target id, prove
// the copy, retarget the mapping, then remove the original. The order matters
// — deleting first would lose an unrecomputable vector if anything after it
// failed.
func (s *RepairService) applyItem(ctx context.Context, item RepairItem) error {
	if !item.NeedsCopy() {
		// The mapping is already authoritative. Preserving a correct mapping
		// is cheaper and safer than rewriting it.
		return s.repo.SetItemState(ctx, item, RepairItemCleaned, "")
	}
	target := *item.TargetNumericID
	collections := copyableCollections(item)

	for _, collection := range collections {
		for _, old := range item.OldNumericIDs {
			if old == target {
				continue
			}
			if err := s.mover.CopyPointVerified(ctx, collection, old, target); err != nil {
				return err
			}
		}
	}
	if err := s.repo.SetItemState(ctx, item, RepairItemCopied, ""); err != nil {
		return err
	}
	if err := s.repo.RetargetMapping(ctx, item.Namespace, item.EntityType, item.StringID, target); err != nil {
		return err
	}
	for _, collection := range collections {
		for _, old := range item.OldNumericIDs {
			if old == target {
				continue
			}
			if err := s.mover.DeletePoint(ctx, collection, old); err != nil {
				return err
			}
		}
	}
	return s.repo.SetItemState(ctx, item, RepairItemCleaned, "")
}

// VerifyReport is the outcome of the post-apply verification.
type VerifyReport struct {
	RunID   int64
	Checked int
	// Unmoved holds every item that failed verification — a target point that
	// never landed as much as an old point that was never cleaned.
	Unmoved   []RepairItem
	Remaining []RepairItem
	// Problems explains what failed, so the operator is not left comparing
	// point ids by hand to work out why the gate refused.
	Problems []string
}

// Verify proves the run before the fleet is unlocked: every manifest tuple is
// on its target id and every old point is gone.
func (s *RepairService) Verify(ctx context.Context, runID int64) (*VerifyReport, error) {
	items, err := s.repo.ListItems(ctx, runID)
	if err != nil {
		return nil, err
	}
	run, err := s.repo.GetRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	report := &VerifyReport{RunID: runID, Checked: len(items)}
	// A namespace whose mappings moved is only repaired once its sparse
	// vectors are rebuilt, because those coordinates encode subject numeric
	// ids. Skipping the rebuild leaves the namespace serving stale vectors.
	if s.rebuild != nil {
		for _, namespace := range unrebuiltNamespaces(items, run.RebuiltNamespaces) {
			report.Problems = append(report.Problems,
				fmt.Sprintf("%s: sparse vectors were never rebuilt", namespace))
		}
	}
	for _, item := range items {
		// Both states mean apply is done with the item; `verified` additionally
		// means a previous verification pass already confirmed it.
		if item.State != RepairItemCleaned && item.State != RepairItemVerified {
			report.Remaining = append(report.Remaining, item)
			continue
		}
		problems := verifyItem(ctx, s.mover, item)
		if len(problems) > 0 {
			report.Unmoved = append(report.Unmoved, item)
			report.Problems = append(report.Problems, problems...)
			continue
		}
		// `cleaned` means apply finished with the item; `verified` means this
		// independent gate confirmed it. Recording the distinction is what
		// makes the item-level record match the run-level one.
		if err := s.repo.SetItemState(ctx, item, RepairItemVerified, ""); err != nil {
			return nil, err
		}
	}

	if len(report.Remaining) == 0 && len(report.Unmoved) == 0 && len(report.Problems) == 0 {
		if err := s.repo.SetRunState(ctx, runID, RepairRunComplete, ""); err != nil {
			return nil, err
		}
		return report, nil
	}
	// Names the counts, not a cause. Unmoved collects every item that failed
	// verification, and "target point is missing" lands there just as "the old
	// point is still present" does — asserting the latter sends the operator
	// looking for leftovers when the collection may be empty. The per-item
	// Problems lines above already say which it was.
	message := fmt.Sprintf("%d item(s) unfinished, %d item(s) failed verification",
		len(report.Remaining), len(report.Unmoved))
	if len(report.Problems) > 0 {
		message += ": " + report.Problems[0]
	}
	if err := s.repo.SetRunState(ctx, runID, RepairRunFailed, message); err != nil {
		return nil, err
	}
	slog.WarnContext(ctx, "idmap repair verification failed", "run_id", runID, "detail", message)
	return report, nil
}

// Resume continues a failed or interrupted run from durable item state. It
// re-enters Apply, which skips every item already cleaned.
func (s *RepairService) Resume(ctx context.Context, runID int64) error {
	run, err := s.repo.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if run.State == RepairRunComplete {
		return nil
	}
	if run.State == RepairRunAudited {
		return fmt.Errorf("run %d has no snapshots yet; record them before applying", runID)
	}
	return s.Apply(ctx, runID)
}

// QuarantineReport lists everything blocking a run, so the operator sees the
// full set rather than fixing one item at a time.
func (s *RepairService) QuarantineReport(ctx context.Context, runID int64) ([]RepairItem, error) {
	return s.repo.ListItemsInState(ctx, runID, RepairItemQuarantined)
}

func filterState(items []RepairItem, state RepairItemState) []RepairItem {
	var out []RepairItem
	for _, item := range items {
		if item.State == state {
			out = append(out, item)
		}
	}
	return out
}

// copyableCollections returns the collections apply moves points in.
//
// Only dense vectors are copied: sparse coordinates encode subject numeric
// ids, so once mappings move the whole sparse vector is stale and the full
// recompute rebuilds it. Copying them first would do work the rebuild
// immediately discards, on the largest collections in the namespace.
//
// A manifest written before the dense set was recorded falls back to every
// observed collection, so an older run still repairs something.
func copyableCollections(item RepairItem) []string {
	dense, ok := item.Sources["dense_collections"].(map[string]any)
	if !ok || len(dense) == 0 {
		return itemCollections(item)
	}
	out := make([]string, 0, len(dense))
	for collection := range dense {
		out = append(out, collection)
	}
	sort.Strings(out)
	return out
}

// itemCollections reads the collections the audit observed this identity in.
func itemCollections(item RepairItem) []string {
	raw, ok := item.Sources["collections"].(map[string]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for collection := range raw {
		out = append(out, collection)
	}
	sort.Strings(out)
	return out
}

func sortedNamespaces(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for namespace := range set {
		out = append(out, namespace)
	}
	sort.Strings(out)
	return out
}
