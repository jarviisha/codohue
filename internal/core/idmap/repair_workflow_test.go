package idmap

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// Fakes for the orchestration entry points. They are separate from the fakes
// in repair_service_test.go, which model a run that is already applying: these
// have to record mutations and inject failures so the refusals below can be
// shown to mutate nothing.

type workflowStore struct {
	active      *RepairRun
	activeErr   error
	run         *RepairRun
	items       []RepairItem
	quarantined []RepairItem

	createErr error
	created   []RepairItem

	// Error hooks. They exist so the best-effort writes in recordFailure can
	// be made to fail without masking the error that triggered them.
	getRunErr       error
	listItemsErr    error
	setItemStateErr error
	setRunStateErr  error
	snapshotErr     error

	itemStates map[string]RepairItemState
	itemErrors map[string]string
	runState   RepairRunState
	runError   string

	retargets []string
	rebuilt   []string
	snapshots map[string]string
	pgRef     string

	next int64
}

func newWorkflowStore(run *RepairRun, items []RepairItem) *workflowStore {
	return &workflowStore{
		run:        run,
		items:      items,
		itemStates: map[string]RepairItemState{},
		itemErrors: map[string]string{},
	}
}

func (f *workflowStore) ActiveRun(context.Context) (*RepairRun, error) {
	return f.active, f.activeErr
}

func (f *workflowStore) CreateRun(_ context.Context, items []RepairItem, _ time.Time) (*RepairRun, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	f.created = items
	if f.run == nil {
		f.run = &RepairRun{ID: 1, State: RepairRunAudited}
	}
	f.run.ManifestHash = ManifestHash(items)
	return f.run, nil
}

func (f *workflowStore) GetRun(context.Context, int64) (*RepairRun, error) {
	return f.run, f.getRunErr
}

func (f *workflowStore) ListItems(context.Context, int64) ([]RepairItem, error) {
	if f.listItemsErr != nil {
		return nil, f.listItemsErr
	}
	return f.items, nil
}

func (f *workflowStore) ListItemsInState(_ context.Context, _ int64, _ ...RepairItemState) ([]RepairItem, error) {
	return f.quarantined, nil
}

func (f *workflowStore) NextNumericID(context.Context) (int64, error) {
	f.next++
	return 9000 + f.next, nil
}

func (f *workflowStore) RecordRebuiltNamespace(_ context.Context, _ int64, namespace string) error {
	f.rebuilt = append(f.rebuilt, namespace)
	return nil
}

func (f *workflowStore) RecordSnapshots(_ context.Context, _ int64, pgRef string, refs map[string]string) error {
	if f.snapshotErr != nil {
		return f.snapshotErr
	}
	f.pgRef = pgRef
	f.snapshots = refs
	return nil
}

func (f *workflowStore) RetargetMapping(_ context.Context, namespace, entityType, stringID string, numericID int64) error {
	f.retargets = append(f.retargets, key4(namespace, entityType, stringID, numericID))
	return nil
}

func (f *workflowStore) SetItemState(_ context.Context, item RepairItem, state RepairItemState, errMessage string) error {
	if f.setItemStateErr != nil {
		return f.setItemStateErr
	}
	f.itemStates[item.StringID] = state
	if errMessage != "" {
		f.itemErrors[item.StringID] = errMessage
	}
	return nil
}

func (f *workflowStore) SetRunState(_ context.Context, _ int64, state RepairRunState, errMessage string) error {
	if f.setRunStateErr != nil {
		return f.setRunStateErr
	}
	f.runState = state
	f.runError = errMessage
	return nil
}

func key4(namespace, entityType, stringID string, numericID int64) string {
	return namespace + "/" + entityType + "/" + stringID + "->" + itoa(numericID)
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	var digits []byte
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	return string(digits)
}

// workflowMover records the order of copies and deletes. The order is the
// whole safety property of applyItem: a delete that runs before its copy loses
// a vector nothing can recompute.
type workflowMover struct {
	ops     []string
	copyErr error
	delErr  error
}

func (m *workflowMover) CopyPointVerified(_ context.Context, collection string, from, to int64) error {
	m.ops = append(m.ops, "copy "+collection+" "+itoa(from)+"->"+itoa(to))
	return m.copyErr
}

func (m *workflowMover) DeletePoint(_ context.Context, collection string, id int64) error {
	m.ops = append(m.ops, "delete "+collection+" "+itoa(id))
	return m.delErr
}

func (m *workflowMover) PointAbsent(context.Context, string, int64) (bool, error) { return true, nil }

func (m *workflowMover) InspectPoint(context.Context, string, int64) (InspectedPoint, bool, error) {
	return InspectedPoint{}, false, nil
}

// passthroughFence stands in for the global exclusive lease. It records that
// the callback ran inside it, because Apply must never touch a store outside.
type passthroughFence struct {
	entered bool
}

func (f *passthroughFence) WithGlobalExclusive(ctx context.Context, fn func(context.Context) error) error {
	f.entered = true
	return fn(ctx)
}

type recordingRebuilder struct {
	namespaces []string
	err        error
}

func (r *recordingRebuilder) RebuildSparse(_ context.Context, namespace string) error {
	r.namespaces = append(r.namespaces, namespace)
	return r.err
}

type stubEvidence struct {
	namespaces  []string
	byNamespace map[string]*NamespaceEvidence
	listErr     error
	evidenceErr error
}

func (s *stubEvidence) Namespaces(context.Context) ([]string, error) {
	return s.namespaces, s.listErr
}

func (s *stubEvidence) Evidence(_ context.Context, namespace string) (*NamespaceEvidence, error) {
	if s.evidenceErr != nil {
		return nil, s.evidenceErr
	}
	return s.byNamespace[namespace], nil
}

func int64p(v int64) *int64 { return &v }

// denseSources builds the Sources map an audited item carries, so applyItem
// copies in the dense collection only.
func denseSources(collections ...string) map[string]any {
	dense := map[string]any{}
	all := map[string]any{}
	for _, c := range collections {
		dense[c] = true
		all[c] = true
	}
	return map[string]any{"dense_collections": dense, "collections": all}
}

func pendingItem(stringID string, old []int64, target int64, collections ...string) RepairItem {
	return RepairItem{
		RunID:           1,
		Namespace:       "ns",
		EntityType:      "object",
		StringID:        stringID,
		OldNumericIDs:   old,
		TargetNumericID: int64p(target),
		Sources:         denseSources(collections...),
		State:           RepairItemPending,
	}
}

// snapshottingRun is the state Apply expects: audited, snapshots recorded, and
// a manifest hash matching the items the store will hand back.
func snapshottingRun(items []RepairItem, refs map[string]string) *RepairRun {
	return &RepairRun{
		ID:                 1,
		State:              RepairRunSnapshotting,
		PGSnapshotRef:      "pg-dump-2026-08-26",
		QdrantSnapshotRefs: refs,
		ManifestHash:       ManifestHash(items),
	}
}

// --- Audit ---------------------------------------------------------------

func TestAudit_FreezesAManifestAndCountsTheDecisions(t *testing.T) {
	store := newWorkflowStore(nil, nil)
	svc := &RepairService{
		repo: store,
		evidence: &stubEvidence{
			namespaces: []string{"ns"},
			byNamespace: map[string]*NamespaceEvidence{
				"ns": evidenceFor("ns",
					map[string]map[string]int64{"object": {"o1": 10, "o2": 20}},
					CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 11, StringID: "o1"},
					CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 20, StringID: "o2"},
				),
			},
		},
		now: time.Now,
	}

	report, err := svc.Audit(context.Background())
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if report.Total != 2 {
		t.Fatalf("Total = %d, want 2", report.Total)
	}
	// o1 is mapped to 10 but its point sits at 11, so it needs a copy; o2 is
	// already authoritative and must not be counted as work.
	if report.NeedsRepair != 1 {
		t.Errorf("NeedsRepair = %d, want 1", report.NeedsRepair)
	}
	if report.Resolved != 2 {
		t.Errorf("Resolved = %d, want 2", report.Resolved)
	}
	if len(report.Quarantined) != 0 {
		t.Errorf("Quarantined = %v, want none", report.Quarantined)
	}
	if report.ManifestHash != ManifestHash(store.created) {
		t.Errorf("report hash %q does not match the recorded manifest", report.ManifestHash)
	}
}

// The audit is read-only by contract, so an unresolvable identity comes back
// on the report rather than being decided.
func TestAudit_ReportsQuarantinedItemsWithoutDeciding(t *testing.T) {
	store := newWorkflowStore(nil, nil)
	svc := &RepairService{
		repo: store,
		evidence: &stubEvidence{
			namespaces: []string{"ns"},
			byNamespace: map[string]*NamespaceEvidence{
				// Two points in one collection claim the same string id, so
				// nothing can say which numeric id is authoritative.
				"ns": evidenceFor("ns",
					map[string]map[string]int64{"object": {"o1": 10}},
					CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 11, StringID: "o1"},
					CollectionEvidence{Collection: "ns_objects_dense", EntityType: "object", NumericID: 12, StringID: "o1"},
				),
			},
		},
		now: time.Now,
	}

	report, err := svc.Audit(context.Background())
	if err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if len(report.Quarantined) != 1 {
		t.Fatalf("Quarantined = %d item(s), want 1", len(report.Quarantined))
	}
	if report.NeedsRepair != 0 {
		t.Errorf("NeedsRepair = %d, want 0 — a quarantined item is not repairable work", report.NeedsRepair)
	}
	if len(store.retargets) != 0 {
		t.Errorf("audit retargeted %v; it must mutate nothing", store.retargets)
	}
}

// Resolving a quarantined item changes the evidence, so the operator re-audits.
// An `audited` run has moved nothing, and it never reaches a terminal state on
// its own — without superseding it, it would block every later audit forever.
func TestAudit_SupersedesAPreMutationRun(t *testing.T) {
	store := newWorkflowStore(nil, nil)
	store.active = &RepairRun{ID: 41, State: RepairRunAudited}
	svc := &RepairService{
		repo:     store,
		evidence: &stubEvidence{namespaces: []string{}},
		now:      time.Now,
	}

	if _, err := svc.Audit(context.Background()); err != nil {
		t.Fatalf("Audit: %v", err)
	}
	if store.runState != RepairRunFailed {
		t.Errorf("superseded run state = %q, want failed", store.runState)
	}
	if !strings.Contains(store.runError, "superseded") {
		t.Errorf("superseded run error = %q, want it to say why", store.runError)
	}
}

// Once apply has begun, a second audit would hand the operator a manifest for
// a fleet that is already half-moved.
func TestAudit_RefusesOnceMutationHasBegun(t *testing.T) {
	store := newWorkflowStore(nil, nil)
	store.active = &RepairRun{ID: 42, State: RepairRunApplying}
	svc := &RepairService{repo: store, evidence: &stubEvidence{}, now: time.Now}

	_, err := svc.Audit(context.Background())
	if !errors.Is(err, ErrRepairRunActive) {
		t.Fatalf("Audit error = %v, want ErrRepairRunActive", err)
	}
	if store.runState != "" {
		t.Errorf("refused audit moved the run to %q; it must leave it alone", store.runState)
	}
}

// A partial inventory would freeze a manifest missing whatever the failing
// namespace holds, and apply trusts the manifest completely.
func TestAudit_EvidenceFailureStopsTheRun(t *testing.T) {
	store := newWorkflowStore(nil, nil)
	svc := &RepairService{
		repo:     store,
		evidence: &stubEvidence{namespaces: []string{"ns"}, evidenceErr: errors.New("qdrant unreachable")},
		now:      time.Now,
	}

	_, err := svc.Audit(context.Background())
	if err == nil {
		t.Fatal("Audit succeeded despite unreadable evidence")
	}
	if !strings.Contains(err.Error(), "ns") {
		t.Errorf("error %q does not name the namespace that failed", err)
	}
	if store.created != nil {
		t.Error("a manifest was recorded from incomplete evidence")
	}
}

// --- PrepareSnapshots ----------------------------------------------------

func TestPrepareSnapshots_RecordsTheRefsAndAdvancesTheRun(t *testing.T) {
	store := newWorkflowStore(&RepairRun{ID: 1, State: RepairRunAudited}, nil)
	svc := &RepairService{repo: store, now: time.Now}

	refs := map[string]string{"ns_objects_dense": "snap-1"}
	if err := svc.PrepareSnapshots(context.Background(), 1, "pg-dump", refs); err != nil {
		t.Fatalf("PrepareSnapshots: %v", err)
	}
	if store.pgRef != "pg-dump" {
		t.Errorf("pg ref = %q, want pg-dump", store.pgRef)
	}
	if store.snapshots["ns_objects_dense"] != "snap-1" {
		t.Errorf("qdrant refs = %v, want the collection recorded", store.snapshots)
	}
	if store.runState != RepairRunSnapshotting {
		t.Errorf("run state = %q, want snapshotting", store.runState)
	}
}

// Snapshots taken after apply started describe a fleet that has already moved,
// so recording them would make the run look recoverable when it is not.
func TestPrepareSnapshots_RefusesOutsideTheAuditWindow(t *testing.T) {
	store := newWorkflowStore(&RepairRun{ID: 1, State: RepairRunApplying}, nil)
	svc := &RepairService{repo: store, now: time.Now}

	err := svc.PrepareSnapshots(context.Background(), 1, "pg-dump", map[string]string{"c": "s"})
	if err == nil {
		t.Fatal("PrepareSnapshots accepted refs for a run that is already applying")
	}
	if store.pgRef != "" {
		t.Errorf("refs were recorded anyway: %q", store.pgRef)
	}
}

// --- Apply ---------------------------------------------------------------

// A read-only audit deployment wires no fence. Apply deletes points, so it must
// refuse rather than run unfenced.
func TestApply_RequiresTheGlobalFence(t *testing.T) {
	svc := &RepairService{repo: newWorkflowStore(nil, nil), now: time.Now}

	err := svc.Apply(context.Background(), 1)
	if err == nil || !strings.Contains(err.Error(), "fence") {
		t.Fatalf("Apply error = %v, want it to name the missing fence", err)
	}
}

func TestApply_MovesEveryItemThenRebuildsSparse(t *testing.T) {
	items := []RepairItem{pendingItem("o1", []int64{11}, 10, "ns_objects_dense")}
	store := newWorkflowStore(snapshottingRun(items, map[string]string{"ns_objects_dense": "snap-1"}), items)
	mover := &workflowMover{}
	rebuilder := &recordingRebuilder{}
	fence := &passthroughFence{}
	svc := &RepairService{repo: store, mover: mover, rebuild: rebuilder, fence: fence, now: time.Now}

	if err := svc.Apply(context.Background(), 1); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !fence.entered {
		t.Error("Apply ran without entering the global exclusive lease")
	}
	// Copy strictly before delete: the reverse order loses the vector if
	// anything in between fails.
	want := []string{"copy ns_objects_dense 11->10", "delete ns_objects_dense 11"}
	if !equalOps(mover.ops, want) {
		t.Errorf("ops = %v, want %v", mover.ops, want)
	}
	if len(store.retargets) != 1 || store.retargets[0] != "ns/object/o1->10" {
		t.Errorf("retargets = %v, want the mapping moved to the target", store.retargets)
	}
	if store.itemStates["o1"] != RepairItemCleaned {
		t.Errorf("item state = %q, want cleaned", store.itemStates["o1"])
	}
	if len(rebuilder.namespaces) != 1 || rebuilder.namespaces[0] != "ns" {
		t.Errorf("rebuilt = %v, want ns", rebuilder.namespaces)
	}
	if len(store.rebuilt) != 1 || store.rebuilt[0] != "ns" {
		t.Errorf("recorded rebuilds = %v, want ns recorded as it completed", store.rebuilt)
	}
	if store.runState != RepairRunVerifying {
		t.Errorf("run state = %q, want verifying", store.runState)
	}
}

// The operator reviewed a specific set of decisions. Applying a different one
// silently is the failure this whole workflow exists to prevent.
func TestApply_RefusesWhenTheManifestChanged(t *testing.T) {
	items := []RepairItem{pendingItem("o1", []int64{11}, 10, "ns_objects_dense")}
	run := snapshottingRun(items, map[string]string{"ns_objects_dense": "snap-1"})
	run.ManifestHash = "a-hash-from-a-different-item-set"
	store := newWorkflowStore(run, items)
	mover := &workflowMover{}
	svc := &RepairService{repo: store, mover: mover, rebuild: &recordingRebuilder{}, fence: &passthroughFence{}, now: time.Now}

	err := svc.Apply(context.Background(), 1)
	if !errors.Is(err, ErrManifestChanged) {
		t.Fatalf("Apply error = %v, want ErrManifestChanged", err)
	}
	assertNothingMutated(t, store, mover)
}

func TestApply_RefusesWhileAnItemIsQuarantined(t *testing.T) {
	blocked := pendingItem("o2", []int64{21}, 20, "ns_objects_dense")
	blocked.State = RepairItemQuarantined
	blocked.Error = "two points claim o2"
	items := []RepairItem{pendingItem("o1", []int64{11}, 10, "ns_objects_dense"), blocked}
	store := newWorkflowStore(snapshottingRun(items, map[string]string{"ns_objects_dense": "snap-1"}), items)
	mover := &workflowMover{}
	svc := &RepairService{repo: store, mover: mover, rebuild: &recordingRebuilder{}, fence: &passthroughFence{}, now: time.Now}

	err := svc.Apply(context.Background(), 1)
	if !errors.Is(err, ErrQuarantinedItems) {
		t.Fatalf("Apply error = %v, want ErrQuarantinedItems", err)
	}
	// The refusal names the blocking item, so the operator is not left
	// searching the manifest for it.
	if !strings.Contains(err.Error(), "o2") {
		t.Errorf("error %q does not name the quarantined item", err)
	}
	// The resolvable item must not have moved either: the run is refused whole.
	assertNothingMutated(t, store, mover)
}

func TestApply_RefusesWithoutAPostgresSnapshot(t *testing.T) {
	items := []RepairItem{pendingItem("o1", []int64{11}, 10, "ns_objects_dense")}
	run := snapshottingRun(items, map[string]string{"ns_objects_dense": "snap-1"})
	run.PGSnapshotRef = ""
	store := newWorkflowStore(run, items)
	mover := &workflowMover{}
	svc := &RepairService{repo: store, mover: mover, rebuild: &recordingRebuilder{}, fence: &passthroughFence{}, now: time.Now}

	if err := svc.Apply(context.Background(), 1); !errors.Is(err, ErrSnapshotsRequired) {
		t.Fatalf("Apply error = %v, want ErrSnapshotsRequired", err)
	}
	assertNothingMutated(t, store, mover)
}

// Coverage is per collection, not "at least one". A run spanning two
// collections that recorded a single snapshot would mutate the other with no
// way back, and would report success doing it.
func TestApply_RefusesWhenACollectionHasNoSnapshot(t *testing.T) {
	items := []RepairItem{
		pendingItem("o1", []int64{11}, 10, "ns_objects_dense"),
		pendingItem("s1", []int64{31}, 30, "ns_subjects_dense"),
	}
	store := newWorkflowStore(snapshottingRun(items, map[string]string{"ns_objects_dense": "snap-1"}), items)
	mover := &workflowMover{}
	svc := &RepairService{repo: store, mover: mover, rebuild: &recordingRebuilder{}, fence: &passthroughFence{}, now: time.Now}

	err := svc.Apply(context.Background(), 1)
	if !errors.Is(err, ErrSnapshotsRequired) {
		t.Fatalf("Apply error = %v, want ErrSnapshotsRequired", err)
	}
	if !strings.Contains(err.Error(), "ns_subjects_dense") {
		t.Errorf("error %q does not name the uncovered collection", err)
	}
	assertNothingMutated(t, store, mover)
}

// This is the resume path: an interrupted apply left some items cleaned, and
// re-entering must not copy them a second time.
func TestApply_SkipsItemsAlreadyApplied(t *testing.T) {
	done := pendingItem("o1", []int64{11}, 10, "ns_objects_dense")
	done.State = RepairItemCleaned
	pending := pendingItem("o2", []int64{21}, 20, "ns_objects_dense")
	items := []RepairItem{done, pending}
	store := newWorkflowStore(snapshottingRun(items, map[string]string{"ns_objects_dense": "snap-1"}), items)
	mover := &workflowMover{}
	svc := &RepairService{repo: store, mover: mover, rebuild: &recordingRebuilder{}, fence: &passthroughFence{}, now: time.Now}

	if err := svc.Apply(context.Background(), 1); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	for _, op := range mover.ops {
		if strings.Contains(op, "->10") || strings.Contains(op, "dense 11") {
			t.Fatalf("apply touched the already-cleaned item: %v", mover.ops)
		}
	}
	if store.itemStates["o2"] != RepairItemCleaned {
		t.Errorf("pending item state = %q, want cleaned", store.itemStates["o2"])
	}
}

// Recording where the run stopped is what makes it resumable — but it must not
// mask the error that stopped it.
func TestApply_RecordsWhereItStoppedAndReturnsTheCause(t *testing.T) {
	items := []RepairItem{pendingItem("o1", []int64{11}, 10, "ns_objects_dense")}
	store := newWorkflowStore(snapshottingRun(items, map[string]string{"ns_objects_dense": "snap-1"}), items)
	mover := &workflowMover{copyErr: errors.New("qdrant refused the copy")}
	svc := &RepairService{repo: store, mover: mover, rebuild: &recordingRebuilder{}, fence: &passthroughFence{}, now: time.Now}

	err := svc.Apply(context.Background(), 1)
	if err == nil || !strings.Contains(err.Error(), "qdrant refused the copy") {
		t.Fatalf("Apply error = %v, want the underlying cause", err)
	}
	if store.itemStates["o1"] != RepairItemFailed {
		t.Errorf("item state = %q, want failed", store.itemStates["o1"])
	}
	if store.runState != RepairRunFailed {
		t.Errorf("run state = %q, want failed", store.runState)
	}
	// The mapping must not have moved: the point never landed on the target.
	if len(store.retargets) != 0 {
		t.Errorf("retargets = %v, want none after a failed copy", store.retargets)
	}
}

// Sparse coordinates encode subject numeric ids. A rebuild that never ran
// leaves the namespace serving stale vectors, so the run must not reach
// verifying.
func TestApply_RebuildFailureFailsTheRun(t *testing.T) {
	items := []RepairItem{pendingItem("o1", []int64{11}, 10, "ns_objects_dense")}
	store := newWorkflowStore(snapshottingRun(items, map[string]string{"ns_objects_dense": "snap-1"}), items)
	svc := &RepairService{
		repo:    store,
		mover:   &workflowMover{},
		rebuild: &recordingRebuilder{err: errors.New("compute is down")},
		fence:   &passthroughFence{},
		now:     time.Now,
	}

	err := svc.Apply(context.Background(), 1)
	if err == nil || !strings.Contains(err.Error(), "compute is down") {
		t.Fatalf("Apply error = %v, want the rebuild failure", err)
	}
	if store.runState != RepairRunFailed {
		t.Errorf("run state = %q, want failed", store.runState)
	}
	if len(store.rebuilt) != 0 {
		t.Errorf("recorded rebuilds = %v, want none — the rebuild failed", store.rebuilt)
	}
}

// --- applyItem -----------------------------------------------------------

// Preserving a correct mapping is cheaper and safer than rewriting it.
func TestApplyItem_AnAuthoritativeMappingIsOnlyMarkedCleaned(t *testing.T) {
	item := pendingItem("o1", []int64{10}, 10, "ns_objects_dense")
	store := newWorkflowStore(nil, nil)
	mover := &workflowMover{}
	svc := &RepairService{repo: store, mover: mover, now: time.Now}

	if err := svc.applyItem(context.Background(), item); err != nil {
		t.Fatalf("applyItem: %v", err)
	}
	if len(mover.ops) != 0 {
		t.Errorf("ops = %v, want none for an item that needs no copy", mover.ops)
	}
	if len(store.retargets) != 0 {
		t.Errorf("retargets = %v, want none — the mapping is already right", store.retargets)
	}
	if store.itemStates["o1"] != RepairItemCleaned {
		t.Errorf("item state = %q, want cleaned", store.itemStates["o1"])
	}
}

// An identity observed at both its target and a stale id must keep the target
// point: copying it onto itself and then deleting it would destroy the vector.
func TestApplyItem_NeverCopiesOrDeletesTheTargetID(t *testing.T) {
	item := pendingItem("o1", []int64{10, 11}, 10, "ns_objects_dense")
	svc := &RepairService{repo: newWorkflowStore(nil, nil), mover: &workflowMover{}, now: time.Now}
	mover := svc.mover.(*workflowMover)

	if err := svc.applyItem(context.Background(), item); err != nil {
		t.Fatalf("applyItem: %v", err)
	}
	for _, op := range mover.ops {
		if strings.HasSuffix(op, "dense 10") || strings.Contains(op, "10->10") {
			t.Fatalf("apply touched the target id: %v", mover.ops)
		}
	}
}

// The mapping is retargeted only after every copy is proven, and the old
// points are removed only after the mapping points away from them.
func TestApplyItem_OrdersCopyRetargetDelete(t *testing.T) {
	item := pendingItem("o1", []int64{11, 12}, 10, "ns_objects_dense")
	store := newWorkflowStore(nil, nil)
	mover := &workflowMover{delErr: errors.New("delete failed")}
	svc := &RepairService{repo: store, mover: mover, now: time.Now}

	if err := svc.applyItem(context.Background(), item); err == nil {
		t.Fatal("applyItem swallowed the delete failure")
	}
	// Both copies ran and the mapping already moved before the delete was
	// attempted, so a failure here loses nothing.
	want := []string{"copy ns_objects_dense 11->10", "copy ns_objects_dense 12->10", "delete ns_objects_dense 11"}
	if !equalOps(mover.ops, want) {
		t.Errorf("ops = %v, want %v", mover.ops, want)
	}
	if len(store.retargets) != 1 {
		t.Errorf("retargets = %v, want the mapping moved before any delete", store.retargets)
	}
	if store.itemStates["o1"] != RepairItemCopied {
		t.Errorf("item state = %q, want copied — the item never reached cleaned", store.itemStates["o1"])
	}
}

// --- Resume --------------------------------------------------------------

func TestResume_ACompletedRunIsANoop(t *testing.T) {
	store := newWorkflowStore(&RepairRun{ID: 1, State: RepairRunComplete}, nil)
	// No fence: reaching Apply at all would fail loudly here.
	svc := &RepairService{repo: store, now: time.Now}

	if err := svc.Resume(context.Background(), 1); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if store.runState != "" {
		t.Errorf("Resume moved a complete run to %q", store.runState)
	}
}

// A run that never recorded snapshots has no recovery path, so resume must
// send the operator back to PrepareSnapshots rather than into Apply.
func TestResume_RefusesBeforeSnapshotsWereRecorded(t *testing.T) {
	store := newWorkflowStore(&RepairRun{ID: 1, State: RepairRunAudited}, nil)
	svc := &RepairService{repo: store, fence: &passthroughFence{}, now: time.Now}

	err := svc.Resume(context.Background(), 1)
	if err == nil || !strings.Contains(err.Error(), "snapshots") {
		t.Fatalf("Resume error = %v, want it to name the missing snapshots", err)
	}
}

// Resume is Apply re-entered: the fence is taken again and cleaned items are
// skipped, which is what makes an interrupted run continue rather than restart.
func TestResume_ReentersApplyUnderTheFence(t *testing.T) {
	items := []RepairItem{pendingItem("o1", []int64{11}, 10, "ns_objects_dense")}
	run := snapshottingRun(items, map[string]string{"ns_objects_dense": "snap-1"})
	run.State = RepairRunFailed
	store := newWorkflowStore(run, items)
	fence := &passthroughFence{}
	svc := &RepairService{repo: store, mover: &workflowMover{}, rebuild: &recordingRebuilder{}, fence: fence, now: time.Now}

	if err := svc.Resume(context.Background(), 1); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if !fence.entered {
		t.Error("Resume applied without the global exclusive lease")
	}
	if store.runState != RepairRunVerifying {
		t.Errorf("run state = %q, want verifying", store.runState)
	}
}

// --- QuarantineReport ----------------------------------------------------

// The operator sees the full blocking set rather than fixing one item at a
// time and re-auditing between each.
func TestQuarantineReport_ReturnsEveryBlockingItem(t *testing.T) {
	store := newWorkflowStore(nil, nil)
	store.quarantined = []RepairItem{
		{StringID: "o1", State: RepairItemQuarantined},
		{StringID: "o2", State: RepairItemQuarantined},
	}
	svc := &RepairService{repo: store, now: time.Now}

	items, err := svc.QuarantineReport(context.Background(), 1)
	if err != nil {
		t.Fatalf("QuarantineReport: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d item(s), want 2", len(items))
	}
}

// --- wiring --------------------------------------------------------------

// NewRepairService is the only path cmd/admin uses. rebuild and fence are
// nil-able for a read-only audit deployment, and `now` must be wired or every
// CreateRun would stamp the zero time.
func TestNewRepairService_WiresTheReadOnlyDeployment(t *testing.T) {
	svc := NewRepairService(&RepairRepository{}, &stubEvidence{}, &workflowMover{}, nil, nil)

	if svc.now == nil {
		t.Fatal("now is nil; CreateRun would stamp the zero time")
	}
	if svc.rebuild != nil || svc.fence != nil {
		t.Error("an audit-only deployment must keep rebuild and fence nil")
	}
	if err := svc.Apply(context.Background(), 1); err == nil {
		t.Error("Apply must refuse without a fence")
	}
}

// --- helpers -------------------------------------------------------------

func equalOps(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// assertNothingMutated is the property every Apply refusal shares: the run is
// rejected before a single point or mapping moves.
func assertNothingMutated(t *testing.T, store *workflowStore, mover *workflowMover) {
	t.Helper()
	if len(mover.ops) != 0 {
		t.Errorf("points were moved despite the refusal: %v", mover.ops)
	}
	if len(store.retargets) != 0 {
		t.Errorf("mappings were retargeted despite the refusal: %v", store.retargets)
	}
	if len(store.itemStates) != 0 {
		t.Errorf("item states changed despite the refusal: %v", store.itemStates)
	}
	if store.runState == RepairRunApplying {
		t.Error("the run entered applying despite the refusal")
	}
}

// --- store failures ------------------------------------------------------

// recordFailure's two writes are best-effort: losing the note costs the
// operator a diagnostic, while masking the original error would cost them the
// reason the run stopped.
func TestApply_StoreFailureNeverMasksTheRealCause(t *testing.T) {
	items := []RepairItem{pendingItem("o1", []int64{11}, 10, "ns_objects_dense")}
	store := newWorkflowStore(snapshottingRun(items, map[string]string{"ns_objects_dense": "snap-1"}), items)
	store.setItemStateErr = errors.New("postgres is unreachable")
	svc := &RepairService{
		repo:    store,
		mover:   &workflowMover{},
		rebuild: &recordingRebuilder{},
		fence:   &passthroughFence{},
		now:     time.Now,
	}

	err := svc.Apply(context.Background(), 1)
	if err == nil {
		t.Fatal("Apply succeeded despite an unwritable item state")
	}
	// The item-state write is what failed, and its own error is the cause
	// reported — not swallowed into a generic apply failure.
	if !strings.Contains(err.Error(), "postgres is unreachable") {
		t.Errorf("error = %v, want the underlying store failure", err)
	}
}

// An unreadable manifest cannot be compared against the recorded hash, so the
// run must stop rather than apply an item set nobody reviewed.
func TestApply_UnreadableManifestStopsTheRun(t *testing.T) {
	store := newWorkflowStore(&RepairRun{ID: 1, State: RepairRunSnapshotting}, nil)
	store.listItemsErr = errors.New("postgres is unreachable")
	mover := &workflowMover{}
	svc := &RepairService{repo: store, mover: mover, fence: &passthroughFence{}, now: time.Now}

	if err := svc.Apply(context.Background(), 1); err == nil {
		t.Fatal("Apply ran with an unreadable manifest")
	}
	if len(mover.ops) != 0 {
		t.Errorf("points moved despite the unreadable manifest: %v", mover.ops)
	}
}

func TestAudit_UnreadableActiveRunStopsTheAudit(t *testing.T) {
	store := newWorkflowStore(nil, nil)
	store.activeErr = errors.New("postgres is unreachable")
	svc := &RepairService{repo: store, evidence: &stubEvidence{}, now: time.Now}

	if _, err := svc.Audit(context.Background()); err == nil {
		t.Fatal("Audit proceeded without knowing whether a run is active")
	}
	if store.created != nil {
		t.Error("a manifest was recorded anyway")
	}
}

func TestAudit_NamespaceListFailureStopsTheAudit(t *testing.T) {
	store := newWorkflowStore(nil, nil)
	svc := &RepairService{
		repo:     store,
		evidence: &stubEvidence{listErr: errors.New("postgres is unreachable")},
		now:      time.Now,
	}

	if _, err := svc.Audit(context.Background()); err == nil {
		t.Fatal("Audit proceeded without the namespace list")
	}
	if store.created != nil {
		t.Error("a manifest was recorded from an unknown namespace set")
	}
}

func TestAudit_ManifestWriteFailureIsReported(t *testing.T) {
	store := newWorkflowStore(nil, nil)
	store.createErr = errors.New("postgres is unreachable")
	svc := &RepairService{repo: store, evidence: &stubEvidence{namespaces: []string{}}, now: time.Now}

	if _, err := svc.Audit(context.Background()); err == nil {
		t.Fatal("Audit reported success without recording the manifest")
	}
}

// Superseding is a write. If it fails the old run is still active, so the new
// audit must not proceed as though it had been cleared.
func TestAudit_SupersedeFailureStopsTheAudit(t *testing.T) {
	store := newWorkflowStore(nil, nil)
	store.active = &RepairRun{ID: 41, State: RepairRunAudited}
	store.setRunStateErr = errors.New("postgres is unreachable")
	svc := &RepairService{repo: store, evidence: &stubEvidence{}, now: time.Now}

	_, err := svc.Audit(context.Background())
	if err == nil {
		t.Fatal("Audit proceeded with the previous run still active")
	}
	if !strings.Contains(err.Error(), "41") {
		t.Errorf("error %q does not name the run it failed to supersede", err)
	}
}

func TestPrepareSnapshots_UnreadableRunIsReported(t *testing.T) {
	store := newWorkflowStore(nil, nil)
	store.getRunErr = errors.New("postgres is unreachable")
	svc := &RepairService{repo: store, now: time.Now}

	if err := svc.PrepareSnapshots(context.Background(), 1, "pg", nil); err == nil {
		t.Fatal("PrepareSnapshots accepted refs for an unreadable run")
	}
}

// Reporting success on an unwritten snapshot ref would let apply proceed
// believing it has a recovery path.
func TestPrepareSnapshots_WriteFailureIsReported(t *testing.T) {
	store := newWorkflowStore(&RepairRun{ID: 1, State: RepairRunAudited}, nil)
	store.snapshotErr = errors.New("postgres is unreachable")
	svc := &RepairService{repo: store, now: time.Now}

	if err := svc.PrepareSnapshots(context.Background(), 1, "pg", nil); err == nil {
		t.Fatal("PrepareSnapshots reported success without recording the refs")
	}
	if store.runState == RepairRunSnapshotting {
		t.Error("the run advanced to snapshotting without recorded refs")
	}
}

func TestResume_UnreadableRunIsReported(t *testing.T) {
	store := newWorkflowStore(nil, nil)
	store.getRunErr = errors.New("postgres is unreachable")
	svc := &RepairService{repo: store, fence: &passthroughFence{}, now: time.Now}

	if err := svc.Resume(context.Background(), 1); err == nil {
		t.Fatal("Resume proceeded without reading the run")
	}
}

// --- Verify failure paths -------------------------------------------------

// An item still pending means apply never finished with it, so the gate must
// hold the run open rather than complete it.
func TestVerify_HoldsTheRunOpenWhileItemsRemain(t *testing.T) {
	pending := pendingItem("o1", []int64{11}, 10, "ns_objects_dense")
	store := newWorkflowStore(&RepairRun{ID: 1, State: RepairRunVerifying}, []RepairItem{pending})
	svc := &RepairService{repo: store, mover: &workflowMover{}, now: time.Now}

	report, err := svc.Verify(context.Background(), 1)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(report.Remaining) != 1 {
		t.Fatalf("Remaining = %d, want 1", len(report.Remaining))
	}
	if store.runState != RepairRunFailed {
		t.Errorf("run state = %q, want failed while an item is unfinished", store.runState)
	}
	if !strings.Contains(store.runError, "unfinished") {
		t.Errorf("run error = %q, want it to name the unfinished count", store.runError)
	}
}

func TestVerify_UnreadableManifestIsReported(t *testing.T) {
	store := newWorkflowStore(&RepairRun{ID: 1, State: RepairRunVerifying}, nil)
	store.listItemsErr = errors.New("postgres is unreachable")
	svc := &RepairService{repo: store, mover: &workflowMover{}, now: time.Now}

	if _, err := svc.Verify(context.Background(), 1); err == nil {
		t.Fatal("Verify reported on a manifest it could not read")
	}
}
