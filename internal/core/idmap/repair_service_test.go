package idmap

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
)

func evidenceFor(namespace string, mappings map[string]map[string]int64, points ...CollectionEvidence) *NamespaceEvidence {
	return &NamespaceEvidence{Namespace: namespace, Mappings: mappings, Points: points}
}

// The mapping row is authoritative because it is what every live lookup
// already returns. An identity whose points already sit on that id needs no
// work; one whose points sit elsewhere needs a copy.
func TestAuditNamespace_MappingDecidesTheTarget(t *testing.T) {
	evidence := evidenceFor("ns",
		map[string]map[string]int64{
			"object": {"already-right": 10, "needs-move": 20},
		},
		CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 10, StringID: "already-right"},
		CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 99, StringID: "needs-move"},
	)

	items := auditNamespace(evidence)

	byID := map[string]RepairItem{}
	for _, item := range items {
		byID[item.StringID] = item
	}
	settled := byID["already-right"]
	if !settled.Resolved() || settled.NeedsCopy() {
		t.Errorf("a point already on its mapped id needs no repair: %+v", settled)
	}
	moving := byID["needs-move"]
	if !moving.Resolved() || !moving.NeedsCopy() {
		t.Errorf("a point on the wrong id must be repaired: %+v", moving)
	}
	if *moving.TargetNumericID != 20 {
		t.Errorf("target = %d, want the mapped id 20", *moving.TargetNumericID)
	}
}

// Two points in ONE collection claiming the same identity means the
// authoritative vector cannot be chosen. Picking either would silently discard
// a vector that may not be recomputable, so the audit refuses.
func TestAuditNamespace_AmbiguousPointsAreQuarantined(t *testing.T) {
	evidence := evidenceFor("ns",
		map[string]map[string]int64{"object": {"o1": 10}},
		CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 10, StringID: "o1"},
		CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 11, StringID: "o1"},
	)

	items := auditNamespace(evidence)

	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].State != RepairItemQuarantined {
		t.Errorf("state = %s, want quarantined", items[0].State)
	}
	if items[0].Resolved() {
		t.Error("a quarantined item must not count as resolved")
	}
}

// The same numeric id appearing in two DIFFERENT collections is normal — the
// sparse and dense views of one object share an id — and must not be confused
// with ambiguity.
func TestAuditNamespace_SameIDAcrossCollectionsIsNotAmbiguous(t *testing.T) {
	evidence := evidenceFor("ns",
		map[string]map[string]int64{"object": {"o1": 10}},
		CollectionEvidence{Collection: "ns_objects", EntityType: "object", NumericID: 10, StringID: "o1"},
		CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 10, StringID: "o1"},
	)

	items := auditNamespace(evidence)

	if len(items) != 1 || items[0].State == RepairItemQuarantined {
		t.Fatalf("one object across two collections must resolve cleanly: %+v", items)
	}
}

// A point whose payload carries no logical id cannot be tied to a mapping at
// all. Skipping it would leave it behind silently; the run is blocked instead.
func TestAuditNamespace_UnlabeledPointBlocksTheRun(t *testing.T) {
	evidence := evidenceFor("ns",
		map[string]map[string]int64{},
		CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 42, StringID: ""},
	)

	items := auditNamespace(evidence)

	if len(items) != 1 || items[0].State != RepairItemQuarantined {
		t.Fatalf("an unlabeled point must be quarantined: %+v", items)
	}
}

// Points for an identity with no mapping row have no authoritative id to move
// onto — minting one would invent an identity the rest of the system never
// agreed to.
func TestAuditNamespace_PointsWithoutAMappingAreQuarantined(t *testing.T) {
	evidence := evidenceFor("ns",
		map[string]map[string]int64{"object": {}},
		CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 42, StringID: "orphan"},
	)

	items := auditNamespace(evidence)

	if len(items) != 1 || items[0].State != RepairItemQuarantined {
		t.Fatalf("an unmapped identity must be quarantined: %+v", items)
	}
}

// A mapping with no points is still part of the logical set, so verification
// covers it rather than silently ignoring identities that happen to be
// vectorless.
func TestAuditNamespace_MappingWithoutPointsIsStillAudited(t *testing.T) {
	evidence := evidenceFor("ns", map[string]map[string]int64{"subject": {"u1": 5}})

	items := auditNamespace(evidence)

	if len(items) != 1 || items[0].StringID != "u1" {
		t.Fatalf("expected the vectorless mapping in the manifest: %+v", items)
	}
	if items[0].NeedsCopy() {
		t.Error("an identity with no points needs no copy")
	}
}

// Subjects and objects are separate identity spaces: the same string in both
// is two identities, and merging them would fuse a user with a document.
func TestAuditNamespace_EntityTypesAreSeparateIdentities(t *testing.T) {
	evidence := evidenceFor("ns",
		map[string]map[string]int64{
			"subject": {"shared": 1},
			"object":  {"shared": 2},
		},
		CollectionEvidence{Collection: "ns_subjects", EntityType: "subject", NumericID: 1, StringID: "shared"},
		CollectionEvidence{Collection: "ns_objects", EntityType: "object", NumericID: 2, StringID: "shared"},
	)

	items := auditNamespace(evidence)

	if len(items) != 2 {
		t.Fatalf("expected two distinct identities, got %d", len(items))
	}
	for _, item := range items {
		if item.State == RepairItemQuarantined {
			t.Errorf("cross-entity reuse is legitimate, not ambiguous: %+v", item)
		}
	}
}

// The manifest hash covers the decisions, not just the identities: if a target
// changed between audit and apply, apply must notice.
func TestManifestHash_CoversDecisionsAndIsOrderIndependent(t *testing.T) {
	ten, eleven := int64(10), int64(11)
	base := []RepairItem{
		{Namespace: "ns", EntityType: "object", StringID: "o1", OldNumericIDs: []int64{9}, TargetNumericID: &ten, State: RepairItemPending},
		{Namespace: "ns", EntityType: "subject", StringID: "u1", OldNumericIDs: []int64{1}, TargetNumericID: &eleven, State: RepairItemPending},
	}
	reordered := []RepairItem{base[1], base[0]}

	if ManifestHash(base) != ManifestHash(reordered) {
		t.Error("hash must not depend on item order")
	}

	retargeted := append([]RepairItem(nil), base...)
	other := int64(12)
	retargeted[0].TargetNumericID = &other
	if ManifestHash(base) == ManifestHash(retargeted) {
		t.Error("changing a target must change the hash")
	}

	requarantined := append([]RepairItem(nil), base...)
	requarantined[0].State = RepairItemQuarantined
	if ManifestHash(base) == ManifestHash(requarantined) {
		t.Error("changing an item's state must change the hash")
	}
}

// old_numeric_ids order comes from a map iteration upstream, so the hash sorts
// them — otherwise two identical audits would disagree.
func TestManifestHash_IgnoresOldIDOrdering(t *testing.T) {
	target := int64(10)
	a := []RepairItem{{Namespace: "ns", EntityType: "object", StringID: "o1", OldNumericIDs: []int64{9, 8}, TargetNumericID: &target}}
	b := []RepairItem{{Namespace: "ns", EntityType: "object", StringID: "o1", OldNumericIDs: []int64{8, 9}, TargetNumericID: &target}}

	if ManifestHash(a) != ManifestHash(b) {
		t.Error("old numeric id order must not affect the hash")
	}
}

// NeedsCopy is what decides whether points move at all, so a correct mapping
// must never be rewritten.
func TestRepairItem_NeedsCopy(t *testing.T) {
	target := int64(10)
	for _, tc := range []struct {
		name string
		item RepairItem
		want bool
	}{
		{"already on target", RepairItem{OldNumericIDs: []int64{10}, TargetNumericID: &target}, false},
		{"no points at all", RepairItem{TargetNumericID: &target}, false},
		{"one stale id", RepairItem{OldNumericIDs: []int64{9}, TargetNumericID: &target}, true},
		{"target plus a stale id", RepairItem{OldNumericIDs: []int64{9, 10}, TargetNumericID: &target}, true},
		{"quarantined", RepairItem{OldNumericIDs: []int64{9}, State: RepairItemQuarantined}, false},
		{"unresolved", RepairItem{OldNumericIDs: []int64{9}}, false},
	} {
		if got := tc.item.NeedsCopy(); got != tc.want {
			t.Errorf("%s: NeedsCopy = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestItemCollections_SortedAndTolerantOfMissingSources(t *testing.T) {
	item := RepairItem{Sources: map[string]any{"collections": map[string]any{"b": nil, "a": nil}}}
	got := itemCollections(item)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("got %v, want sorted [a b]", got)
	}
	if got := itemCollections(RepairItem{}); got != nil {
		t.Errorf("missing sources must yield no collections, got %v", got)
	}
}

// ─── target-id occupancy ─────────────────────────────────────────────────────

// An identity's mapped numeric id can already hold a point belonging to a
// DIFFERENT identity — a restored snapshot or a direct-id import is enough to
// produce that. Copying onto it would overwrite a vector that may not be
// recomputable, and CopyPointVerified could not catch it: it verifies the copy
// against its source, never that the destination was free.
func TestAuditNamespace_TargetHeldByAnotherIdentityIsFlagged(t *testing.T) {
	evidence := evidenceFor("ns",
		map[string]map[string]int64{
			"object": {"mover": 10, "squatter": 40},
		},
		// "mover" must move onto 10, but 10 currently holds "squatter".
		CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 20, StringID: "mover"},
		CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 10, StringID: "squatter"},
	)

	items := auditNamespace(evidence)

	byID := map[string]RepairItem{}
	for _, item := range items {
		byID[item.StringID] = item
	}
	mover := byID["mover"]
	if conflict := mover.TargetConflictWith(); conflict != "squatter" {
		t.Fatalf("mover target conflict = %q, want squatter", conflict)
	}
	// The squatter itself is moving to 40 and is not in conflict.
	if conflict := byID["squatter"].TargetConflictWith(); conflict != "" {
		t.Errorf("squatter must not be flagged, got %q", conflict)
	}
}

// The common case must stay unflagged: an identity whose mapped id is either
// free or already holds its own point needs no fresh target.
func TestAuditNamespace_UncontestedTargetIsNotFlagged(t *testing.T) {
	evidence := evidenceFor("ns",
		map[string]map[string]int64{"object": {"a": 10, "b": 20}},
		CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 99, StringID: "a"}, // 10 is free
		CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 20, StringID: "b"}, // already home
	)

	for _, item := range auditNamespace(evidence) {
		if conflict := item.TargetConflictWith(); conflict != "" {
			t.Errorf("%s flagged against %q, want no conflict", item.StringID, conflict)
		}
	}
}

// Occupancy is per collection: the same numeric id in a different collection
// belongs to a different vector space and is not a conflict.
func TestAuditNamespace_OccupancyIsScopedToOneCollection(t *testing.T) {
	evidence := evidenceFor("ns",
		map[string]map[string]int64{"object": {"a": 10, "b": 30}},
		CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 99, StringID: "a"},
		// "b" holds 10 in the SPARSE collection; "a" only moves within dense.
		CollectionEvidence{Collection: "ns_objects", EntityType: "object", NumericID: 10, StringID: "b"},
	)

	byID := map[string]RepairItem{}
	for _, item := range auditNamespace(evidence) {
		byID[item.StringID] = item
	}
	if conflict := byID["a"].TargetConflictWith(); conflict != "" {
		t.Errorf("cross-collection id reuse must not be a conflict, got %q", conflict)
	}
}

// An identity already sitting on its target is not in conflict with itself.
func TestAuditNamespace_SelfOccupancyIsNotAConflict(t *testing.T) {
	evidence := evidenceFor("ns",
		map[string]map[string]int64{"object": {"a": 10}},
		CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 10, StringID: "a"},
	)

	items := auditNamespace(evidence)
	if len(items) != 1 || items[0].TargetConflictWith() != "" {
		t.Fatalf("self-occupancy flagged: %+v", items)
	}
	if items[0].NeedsCopy() {
		t.Error("an identity already on its target needs no copy")
	}
}

// fakeRepairStore lets the reassignment logic be exercised without a database.
type fakeRepairStore struct {
	next  int64
	calls int
	err   error
}

func (f *fakeRepairStore) NextNumericID(context.Context) (int64, error) {
	f.calls++
	if f.err != nil {
		return 0, f.err
	}
	f.next++
	return f.next, nil
}

// A contested target is resolved by moving the item to a FRESH id rather than
// by evicting the occupant: the occupant's vector may be as unrecomputable as
// this one's, so neither may be sacrificed for the other.
func TestReassignContestedTargets_MintsAFreshID(t *testing.T) {
	contested := int64(10)
	items := []RepairItem{
		{
			Namespace: "ns", EntityType: "object", StringID: "mover",
			OldNumericIDs: []int64{20}, TargetNumericID: &contested,
			Sources: map[string]any{"target_conflict_with": "squatter"},
		},
	}
	store := &fakeRepairStore{next: 500}

	if err := reassignContestedTargets(context.Background(), store, items); err != nil {
		t.Fatalf("reassign: %v", err)
	}

	if store.calls != 1 {
		t.Errorf("minted %d id(s), want exactly 1", store.calls)
	}
	if got := *items[0].TargetNumericID; got != 501 {
		t.Errorf("target = %d, want the freshly minted 501", got)
	}
	if from := items[0].Sources[sourceTargetReassignedFrom]; from != contested {
		t.Errorf("reassigned_from = %v, want %d recorded for the operator", from, contested)
	}
	// The item must still need a copy — its point is at 20, target is now 501.
	if !items[0].NeedsCopy() {
		t.Error("a reassigned item still has to move its point")
	}
}

// Uncontested items must not consume sequence values: minting for every item
// would churn the id space on a run that has nothing to reassign.
func TestReassignContestedTargets_LeavesUncontestedItemsAlone(t *testing.T) {
	target := int64(10)
	items := []RepairItem{
		{Namespace: "ns", EntityType: "object", StringID: "a", OldNumericIDs: []int64{20}, TargetNumericID: &target},
		{Namespace: "ns", EntityType: "object", StringID: "quarantined", State: RepairItemQuarantined,
			Sources: map[string]any{"target_conflict_with": "someone"}},
	}
	store := &fakeRepairStore{next: 500}

	if err := reassignContestedTargets(context.Background(), store, items); err != nil {
		t.Fatalf("reassign: %v", err)
	}

	if store.calls != 0 {
		t.Errorf("minted %d id(s) for a run with nothing to reassign", store.calls)
	}
	if *items[0].TargetNumericID != 10 {
		t.Errorf("uncontested target changed to %d", *items[0].TargetNumericID)
	}
}

// A reassignment changes what the operator is being asked to approve, so it
// must change the manifest hash.
func TestReassignContestedTargets_ChangesTheManifest(t *testing.T) {
	contested := int64(10)
	items := []RepairItem{{
		Namespace: "ns", EntityType: "object", StringID: "mover",
		OldNumericIDs: []int64{20}, TargetNumericID: &contested,
		Sources: map[string]any{"target_conflict_with": "squatter"},
	}}
	before := ManifestHash(items)

	if err := reassignContestedTargets(context.Background(), &fakeRepairStore{next: 500}, items); err != nil {
		t.Fatalf("reassign: %v", err)
	}

	if ManifestHash(items) == before {
		t.Error("a reassigned target must change the manifest hash")
	}
}

func TestReassignContestedTargets_MintFailureStopsTheAudit(t *testing.T) {
	contested := int64(10)
	items := []RepairItem{{
		Namespace: "ns", EntityType: "object", StringID: "mover",
		OldNumericIDs: []int64{20}, TargetNumericID: &contested,
		Sources: map[string]any{"target_conflict_with": "squatter"},
	}}

	err := reassignContestedTargets(context.Background(), &fakeRepairStore{err: errSentinel}, items)

	if err == nil {
		t.Fatal("a failure to reserve an id must not leave the item on a contested target")
	}
	if *items[0].TargetNumericID != contested {
		t.Error("the target must be left untouched when minting fails")
	}
}

var errSentinel = errors.New("sequence unavailable")

// ─── re-audit after resolving quarantined items ──────────────────────────────

// The runbook tells the operator to resolve every quarantined item and then run
// audit again. That only works if a run which has not mutated anything can be
// superseded — otherwise the first audit wedges the workflow permanently, since
// an `audited` run never reaches a terminal state on its own.
func TestSupersedable_AllowsReAuditBeforeAnyMutation(t *testing.T) {
	for _, state := range []RepairRunState{RepairRunAudited, RepairRunSnapshotting} {
		if !supersedable(state) {
			t.Errorf("state %q has mutated nothing and must not block a re-audit", state)
		}
	}
}

// Once apply has started, points and mappings may already have moved. A new
// audit would build a manifest describing a half-repaired fleet, so the
// operator must resume or verify instead.
func TestSupersedable_RefusesOnceMutationHasBegun(t *testing.T) {
	for _, state := range []RepairRunState{RepairRunApplying, RepairRunVerifying} {
		if supersedable(state) {
			t.Errorf("state %q has begun mutating and must block a re-audit", state)
		}
	}
}

// Terminal runs never reach the supersede check — ActiveRun excludes them — but
// the rule must still read correctly if they do.
func TestSupersedable_TerminalStatesAreNotActive(t *testing.T) {
	for _, state := range []RepairRunState{RepairRunComplete, RepairRunFailed} {
		if !supersedable(state) {
			t.Errorf("terminal state %q must never block a new audit", state)
		}
	}
}

// ─── snapshot coverage ───────────────────────────────────────────────────────

// Apply deletes points that may not be recomputable, so every collection it
// touches needs a recovery point. Accepting one snapshot for a run spanning
// four collections leaves three with no way back — and the operator would have
// no signal, because apply reported success.
func TestMissingSnapshotCollections_NamesEveryUncoveredCollection(t *testing.T) {
	items := []RepairItem{
		{Sources: map[string]any{"collections": map[string]any{"ns_objects": nil, "ns_objects_dense": nil}}},
		{Sources: map[string]any{"collections": map[string]any{"ns_subjects": nil}}},
	}

	missing := missingSnapshotCollections(items, map[string]string{"ns_objects": "snap-a"})

	if len(missing) != 2 {
		t.Fatalf("got %v, want the two uncovered collections", missing)
	}
	// Sorted so the operator sees a stable list across runs.
	if missing[0] != "ns_objects_dense" || missing[1] != "ns_subjects" {
		t.Errorf("got %v, want [ns_objects_dense ns_subjects]", missing)
	}
}

func TestMissingSnapshotCollections_FullCoverageIsAccepted(t *testing.T) {
	items := []RepairItem{
		{Sources: map[string]any{"collections": map[string]any{"ns_objects": nil, "ns_objects_dense": nil}}},
	}

	missing := missingSnapshotCollections(items, map[string]string{
		"ns_objects": "snap-a", "ns_objects_dense": "snap-b",
	})

	if len(missing) != 0 {
		t.Errorf("full coverage reported missing: %v", missing)
	}
}

// A blank reference is not a snapshot — an operator passing an empty value
// must not satisfy the gate.
func TestMissingSnapshotCollections_BlankReferenceIsNotCoverage(t *testing.T) {
	items := []RepairItem{{Sources: map[string]any{"collections": map[string]any{"ns_objects": nil}}}}

	missing := missingSnapshotCollections(items, map[string]string{"ns_objects": "   "})

	if len(missing) != 1 {
		t.Errorf("a blank reference counted as coverage: %v", missing)
	}
}

// A quarantined item blocks apply anyway, but its collections still belong to
// the manifest — the coverage check must not depend on item state.
func TestMissingSnapshotCollections_CoversEveryManifestRow(t *testing.T) {
	items := []RepairItem{
		{State: RepairItemQuarantined, Sources: map[string]any{"collections": map[string]any{"ns_objects": nil}}},
	}

	if missing := missingSnapshotCollections(items, nil); len(missing) != 1 {
		t.Errorf("quarantined rows were excluded from coverage: %v", missing)
	}
}

// A manifest with no recorded collections needs no Qdrant snapshot: there is
// nothing in the vector store to lose.
func TestMissingSnapshotCollections_EmptyManifestNeedsNothing(t *testing.T) {
	if missing := missingSnapshotCollections(nil, nil); len(missing) != 0 {
		t.Errorf("empty manifest demanded snapshots: %v", missing)
	}
}

// ─── verification completeness ───────────────────────────────────────────────

// fakePointMover answers the verification reads without a vector store.
type fakePointMover struct {
	// present maps "collection#id" to the identity and vector hash stored there.
	present map[string]storedPoint
	absent  map[string]bool
	err     error
	reads   int
}

type storedPoint struct {
	stringID   string
	vectorHash string
}

func (f *fakePointMover) CopyPointVerified(context.Context, string, int64, int64) error { return nil }
func (f *fakePointMover) DeletePoint(context.Context, string, int64) error              { return nil }

func (f *fakePointMover) PointAbsent(_ context.Context, collection string, id int64) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.absent[pointKey(collection, id)], nil
}

func (f *fakePointMover) InspectPoint(_ context.Context, collection string, id int64) (stringID, vectorHash string, found bool, err error) {
	f.reads++
	if f.err != nil {
		return "", "", false, f.err
	}
	point, ok := f.present[pointKey(collection, id)]
	return point.stringID, point.vectorHash, ok, nil
}

func pointKey(collection string, id int64) string {
	return collection + "#" + strconv.FormatInt(id, 10)
}

func cleanedItem(target int64) RepairItem {
	return RepairItem{
		Namespace: "ns", EntityType: "object", StringID: "o1",
		OldNumericIDs: []int64{20}, TargetNumericID: &target,
		VectorHash: "vhash", State: RepairItemCleaned,
		Sources: map[string]any{"collections": map[string]any{"ns_objects_dense": nil}},
	}
}

// Verify is the gate before the fleet is unlocked, so it re-reads the target
// rather than trusting that apply's own check ran. A run resumed from a
// manifest whose items were already marked cleaned would otherwise unlock
// without anything ever confirming the vectors are intact.
func TestVerifyItem_ConfirmsTheTargetHoldsTheRightVector(t *testing.T) {
	mover := &fakePointMover{
		present: map[string]storedPoint{"ns_objects_dense#10": {stringID: "o1", vectorHash: "vhash"}},
		absent:  map[string]bool{"ns_objects_dense#20": true},
	}

	problems := verifyItem(context.Background(), mover, cleanedItem(10))

	if len(problems) != 0 {
		t.Fatalf("a correctly repaired item reported %v", problems)
	}
	if mover.reads == 0 {
		t.Error("verification must actually re-read the target point")
	}
}

// A target that is simply not there is the failure this check exists for: the
// old point was deleted and the replacement never landed.
func TestVerifyItem_MissingTargetIsReported(t *testing.T) {
	mover := &fakePointMover{
		present: map[string]storedPoint{},
		absent:  map[string]bool{"ns_objects_dense#20": true},
	}

	problems := verifyItem(context.Background(), mover, cleanedItem(10))

	if len(problems) == 0 {
		t.Fatal("a missing target point must be reported")
	}
	if !strings.Contains(problems[0], "missing") {
		t.Errorf("problem should name the cause: %q", problems[0])
	}
}

// A target holding a different identity's payload means the copy went to the
// wrong place, or something overwrote it afterwards.
func TestVerifyItem_ForeignPayloadIsReported(t *testing.T) {
	mover := &fakePointMover{
		present: map[string]storedPoint{"ns_objects_dense#10": {stringID: "someone-else", vectorHash: "vhash"}},
		absent:  map[string]bool{"ns_objects_dense#20": true},
	}

	problems := verifyItem(context.Background(), mover, cleanedItem(10))

	if len(problems) == 0 || !strings.Contains(problems[0], "someone-else") {
		t.Fatalf("a foreign payload must be reported, got %v", problems)
	}
}

// The vector hash is what proves an unrecomputable vector survived intact.
func TestVerifyItem_AlteredVectorIsReported(t *testing.T) {
	mover := &fakePointMover{
		present: map[string]storedPoint{"ns_objects_dense#10": {stringID: "o1", vectorHash: "different"}},
		absent:  map[string]bool{"ns_objects_dense#20": true},
	}

	problems := verifyItem(context.Background(), mover, cleanedItem(10))

	if len(problems) == 0 || !strings.Contains(problems[0], "vector") {
		t.Fatalf("an altered vector must be reported, got %v", problems)
	}
}

// An old point still present means cleanup did not finish — the fleet would
// unlock with two points claiming one identity.
func TestVerifyItem_SurvivingOldPointIsReported(t *testing.T) {
	mover := &fakePointMover{
		present: map[string]storedPoint{
			"ns_objects_dense#10": {stringID: "o1", vectorHash: "vhash"},
			"ns_objects_dense#20": {stringID: "o1", vectorHash: "vhash"},
		},
		absent: map[string]bool{},
	}

	problems := verifyItem(context.Background(), mover, cleanedItem(10))

	if len(problems) == 0 {
		t.Fatal("a surviving old point must be reported")
	}
}

// An item that never needed a copy has nothing at a new id to check; verifying
// it must not invent a failure.
func TestVerifyItem_ItemThatNeverMovedNeedsNoTargetRead(t *testing.T) {
	mover := &fakePointMover{present: map[string]storedPoint{}}
	settled := cleanedItem(20) // target == its only old id

	problems := verifyItem(context.Background(), mover, settled)

	if len(problems) != 0 {
		t.Fatalf("an unmoved item reported %v", problems)
	}
	if mover.reads != 0 {
		t.Error("an unmoved item needs no target read")
	}
}

// A manifest written before vector hashes were recorded cannot be checked for
// content, but identity and old-point absence still are — degrading is better
// than skipping the whole item.
func TestVerifyItem_MissingRecordedHashSkipsOnlyTheContentCheck(t *testing.T) {
	item := cleanedItem(10)
	item.VectorHash = ""
	mover := &fakePointMover{
		present: map[string]storedPoint{"ns_objects_dense#10": {stringID: "o1", vectorHash: "anything"}},
		absent:  map[string]bool{"ns_objects_dense#20": true},
	}

	problems := verifyItem(context.Background(), mover, item)

	if len(problems) != 0 {
		t.Fatalf("a manifest without a recorded hash reported %v", problems)
	}
}

// ─── dense-only point copying ────────────────────────────────────────────────

// Sparse coordinates encode subject numeric ids, so they cannot be repaired by
// moving a point — the whole vector is stale once mappings change, which is
// why a full recompute follows. Copying them anyway does work the rebuild
// immediately discards, on the largest collections in the namespace.
func TestCopyableCollections_ExcludesSparse(t *testing.T) {
	item := RepairItem{Sources: map[string]any{
		"collections": map[string]any{
			"ns_objects":        nil,
			"ns_objects_dense":  nil,
			"ns_subjects":       nil,
			"ns_subjects_dense": nil,
		},
		"dense_collections": map[string]any{
			"ns_objects_dense":  true,
			"ns_subjects_dense": true,
		},
	}}

	got := copyableCollections(item)

	if len(got) != 2 || got[0] != "ns_objects_dense" || got[1] != "ns_subjects_dense" {
		t.Fatalf("got %v, want only the dense collections", got)
	}
}

// A manifest written before the dense set was recorded must still repair
// something rather than silently copying nothing — falling back to every
// observed collection preserves the old behaviour.
func TestCopyableCollections_FallsBackWhenTheDenseSetIsAbsent(t *testing.T) {
	item := RepairItem{Sources: map[string]any{
		"collections": map[string]any{"ns_objects": nil, "ns_objects_dense": nil},
	}}

	got := copyableCollections(item)

	if len(got) != 2 {
		t.Fatalf("got %v, want the full observed set as a fallback", got)
	}
}

// Verification checks what apply actually moved, so it must use the same
// scope — otherwise it would demand a target point in a sparse collection
// nothing ever copied into.
func TestVerifyItem_ChecksOnlyTheCollectionsApplyCopies(t *testing.T) {
	target := int64(10)
	item := RepairItem{
		Namespace: "ns", EntityType: "object", StringID: "o1",
		OldNumericIDs: []int64{20}, TargetNumericID: &target,
		VectorHash: "vhash", State: RepairItemCleaned,
		Sources: map[string]any{
			"collections":       map[string]any{"ns_objects": nil, "ns_objects_dense": nil},
			"dense_collections": map[string]any{"ns_objects_dense": true},
		},
	}
	mover := &fakePointMover{
		present: map[string]storedPoint{"ns_objects_dense#10": {stringID: "o1", vectorHash: "vhash"}},
		absent:  map[string]bool{"ns_objects_dense#20": true},
	}

	problems := verifyItem(context.Background(), mover, item)

	if len(problems) != 0 {
		t.Fatalf("verification looked outside the copied collections: %v", problems)
	}
	if mover.reads != 1 {
		t.Errorf("read %d point(s), want only the dense one", mover.reads)
	}
}
