package compute

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/qdrant/go-client/qdrant"

	"github.com/jarviisha/codohue/internal/core/nslifecycle"
)

// ─── fakes ───────────────────────────────────────────────────────────────────

type fakeComputeRepo struct {
	subjects      []string
	events        []*RawEvent
	err           error
	subjectEvents map[string][]*RawEvent
}

func (f *fakeComputeRepo) GetActiveSubjects(_ context.Context, _ string) ([]string, error) {
	return f.subjects, f.err
}

func (f *fakeComputeRepo) GetSubjectEvents(_ context.Context, _, subjectID string) ([]*RawEvent, error) {
	if f.subjectEvents != nil {
		return f.subjectEvents[subjectID], f.err
	}
	return f.events, f.err
}

type fakeIDMap struct {
	subjectID   uint64
	subjectErr  error
	objectIDs   map[string]uint64
	nextID      uint64
	objectErrs  map[string]error
	singleCalls int
	batchCalls  int
	batchErr    error
}

func newFakeIDMap() *fakeIDMap {
	return &fakeIDMap{
		subjectID:  1,
		objectIDs:  make(map[string]uint64),
		nextID:     10,
		objectErrs: make(map[string]error),
	}
}

func (f *fakeIDMap) GetOrCreateSubjectID(_ context.Context, _, _ string) (uint64, error) {
	return f.subjectID, f.subjectErr
}

func (f *fakeIDMap) alloc(objectID string) (uint64, error) {
	if err, ok := f.objectErrs[objectID]; ok {
		return 0, err
	}
	if id, ok := f.objectIDs[objectID]; ok {
		return id, nil
	}
	f.nextID++
	f.objectIDs[objectID] = f.nextID
	return f.nextID, nil
}

func (f *fakeIDMap) GetOrCreateObjectID(_ context.Context, objectID, _ string) (uint64, error) {
	f.singleCalls++
	return f.alloc(objectID)
}

func (f *fakeIDMap) GetOrCreateObjectIDs(_ context.Context, objectIDs []string, _ string) (map[string]uint64, error) {
	f.batchCalls++
	if f.batchErr != nil {
		return nil, f.batchErr
	}
	out := make(map[string]uint64, len(objectIDs))
	for _, objectID := range objectIDs {
		id, err := f.alloc(objectID)
		if err != nil {
			return nil, err
		}
		out[objectID] = id
	}
	return out, nil
}

func newTestService(repo computeRepo, idmap idmapService) *Service {
	return &Service{
		repo:     repo,
		idmapSvc: idmap,
		upsertFn: func(_ context.Context, _ *qdrant.UpsertPoints) error { return nil },
	}
}

func TestNewService(t *testing.T) {
	svc := NewService(nil, nil, nil)

	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if svc.upsertFn == nil {
		t.Fatal("expected upsertFn to be initialized")
	}
}

// ─── buildVectors ────────────────────────────────────────────────────────────

func TestBuildVectors_SingleEvent(t *testing.T) {
	now := time.Now().Unix()
	events := []*RawEvent{
		{SubjectID: "u1", ObjectID: "o1", Weight: 5.0, OccurredAt: now},
	}
	svc := newTestService(&fakeComputeRepo{events: events}, newFakeIDMap())

	built, err := svc.buildVectors(context.Background(), "ns", "u1", 0.05)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Event happened now → freshness ≈ 1.0, score ≈ weight.
	if math.Abs(built.scores["o1"]-5.0) > 0.01 {
		t.Errorf("score: got %.4f, want ≈5.0", built.scores["o1"])
	}
	if built.maxTimes["o1"] != now {
		t.Errorf("maxTime: got %d, want %d", built.maxTimes["o1"], now)
	}
}

func TestBuildVectors_TimeDecayApplied(t *testing.T) {
	// Event 10 days ago: score = weight * e^(-lambda * 10)
	tenDaysAgo := time.Now().Add(-10 * 24 * time.Hour).Unix()
	events := []*RawEvent{
		{SubjectID: "u1", ObjectID: "o1", Weight: 1.0, OccurredAt: tenDaysAgo},
	}
	svc := newTestService(&fakeComputeRepo{events: events}, newFakeIDMap())

	lambda := 0.05
	built, err := svc.buildVectors(context.Background(), "ns", "u1", lambda)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := math.Exp(-lambda * 10)
	if math.Abs(built.scores["o1"]-want) > 0.01 {
		t.Errorf("decayed score: got %.6f, want %.6f", built.scores["o1"], want)
	}
}

func TestBuildVectors_MultipleEventsAccumulate(t *testing.T) {
	now := time.Now().Unix()
	events := []*RawEvent{
		{SubjectID: "u1", ObjectID: "o1", Weight: 2.0, OccurredAt: now},
		{SubjectID: "u1", ObjectID: "o1", Weight: 3.0, OccurredAt: now},
		{SubjectID: "u1", ObjectID: "o2", Weight: 1.0, OccurredAt: now},
	}
	svc := newTestService(&fakeComputeRepo{events: events}, newFakeIDMap())

	built, err := svc.buildVectors(context.Background(), "ns", "u1", 0.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// lambda=0 → freshness=1.0 everywhere, built.scores are pure sums.
	if math.Abs(built.scores["o1"]-5.0) > 1e-9 {
		t.Errorf("o1 accumulated score: got %.4f, want 5.0", built.scores["o1"])
	}
	if math.Abs(built.scores["o2"]-1.0) > 1e-9 {
		t.Errorf("o2 score: got %.4f, want 1.0", built.scores["o2"])
	}
}

func TestBuildVectors_ObjectCreatedAtTracked(t *testing.T) {
	now := time.Now().Unix()
	created := now - 1000
	events := []*RawEvent{
		{SubjectID: "u1", ObjectID: "o1", Weight: 1.0, OccurredAt: now, ObjectCreatedAt: &created},
	}
	svc := newTestService(&fakeComputeRepo{events: events}, newFakeIDMap())

	built, err := svc.buildVectors(context.Background(), "ns", "u1", 0.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if built.createdTimes["o1"] != created {
		t.Errorf("createdTime: got %d, want %d", built.createdTimes["o1"], created)
	}
}

func TestBuildVectors_NoEvents_EmptyResult(t *testing.T) {
	svc := newTestService(&fakeComputeRepo{events: nil}, newFakeIDMap())

	built, err := svc.buildVectors(context.Background(), "ns", "u1", 0.05)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(built.scores) != 0 {
		t.Errorf("expected empty scores, got %v", built.scores)
	}
	if len(built.maxTimes) != 0 {
		t.Errorf("expected empty maxTimes, got %v", built.maxTimes)
	}
	if len(built.createdTimes) != 0 {
		t.Errorf("expected empty createdTimes, got %v", built.createdTimes)
	}
	if built.vec == nil {
		t.Fatal("expected non-nil SubjectVector even for empty events")
	}
	if len(built.vec.Indices) != 0 {
		t.Errorf("expected empty indices, got %v", built.vec.Indices)
	}
}

func TestBuildVectors_MaxTimeTracksLatest(t *testing.T) {
	older := time.Now().Add(-2 * time.Hour).Unix()
	newer := time.Now().Add(-1 * time.Hour).Unix()
	events := []*RawEvent{
		{SubjectID: "u1", ObjectID: "o1", Weight: 1.0, OccurredAt: older},
		{SubjectID: "u1", ObjectID: "o1", Weight: 1.0, OccurredAt: newer},
	}
	svc := newTestService(&fakeComputeRepo{events: events}, newFakeIDMap())

	built, err := svc.buildVectors(context.Background(), "ns", "u1", 0.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if built.maxTimes["o1"] != newer {
		t.Errorf("maxTime: got %d, want %d (newer)", built.maxTimes["o1"], newer)
	}
}

// ─── existing accumulation + decay helpers (kept for regression) ──────────────

func TestObjectCooccurrenceAccumulation(t *testing.T) {
	idmap := newFakeIDMap()
	svc := newTestService(&fakeComputeRepo{}, idmap)

	objectAccum := make(map[string]map[uint64]float32)
	scores := map[string]float64{"obj-A": 2.0, "obj-B": 1.5, "obj-C": 0.8}
	objectIDs, err := svc.resolveObjectIDs(context.Background(), "ns", scores)
	if err != nil {
		t.Fatalf("resolveObjectIDs: %v", err)
	}
	svc.accumulateObjectCooccurrence(objectAccum, scores, objectIDs)

	objAID := idmap.objectIDs["obj-A"]
	objBID := idmap.objectIDs["obj-B"]
	objCID := idmap.objectIDs["obj-C"]

	if got := objectAccum["obj-A"][objBID]; math.Abs(float64(got)-1.5) > 1e-6 {
		t.Errorf("obj-A[obj-B] = %v, want 1.5", got)
	}
	if got := objectAccum["obj-A"][objCID]; math.Abs(float64(got)-0.8) > 1e-6 {
		t.Errorf("obj-A[obj-C] = %v, want 0.8", got)
	}
	if _, exists := objectAccum["obj-A"][objAID]; exists {
		t.Error("obj-A should not contain self dimension")
	}
}

func TestTimeFreshnessDecay(t *testing.T) {
	lambda := 0.05
	daysSince := 10.0
	weight := 1.0

	got := weight * math.Exp(-lambda*daysSince)
	want := math.Exp(-0.5)

	if math.Abs(got-want) > 1e-9 {
		t.Errorf("decay = %v, want %v", got, want)
	}
}

func TestBuildSubjectVector_SubjectIDError(t *testing.T) {
	idmap := newFakeIDMap()
	idmap.subjectErr = context.DeadlineExceeded
	svc := newTestService(&fakeComputeRepo{}, idmap)

	_, err := svc.buildSubjectVector(context.Background(), "ns", "u1", map[string]float64{"o1": 1}, map[string]uint64{"o1": 11})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// Resolution moved upstream into resolveObjectIDs; a failure there must still
// sink the subject rather than silently building a vector without it.
func TestResolveObjectIDs_PropagatesBatchError(t *testing.T) {
	idmap := newFakeIDMap()
	idmap.batchErr = context.Canceled
	svc := newTestService(&fakeComputeRepo{}, idmap)

	if _, err := svc.resolveObjectIDs(context.Background(), "ns", map[string]float64{"o1": 1}); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestBuildSubjectVector_UnresolvedObjectIsAnError(t *testing.T) {
	svc := newTestService(&fakeComputeRepo{}, newFakeIDMap())

	_, err := svc.buildSubjectVector(context.Background(), "ns", "u1", map[string]float64{"o1": 1}, map[string]uint64{})
	if err == nil {
		t.Fatal("an object with no resolved numeric id must fail, not be dropped")
	}
}

func TestUpsertSubjectVector_SendsExpectedPayload(t *testing.T) {
	svc := newTestService(&fakeComputeRepo{}, newFakeIDMap())
	var got *qdrant.UpsertPoints
	svc.upsertFn = func(_ context.Context, points *qdrant.UpsertPoints) error {
		got = points
		return nil
	}

	err := svc.upsertSubjectVector(leasedCtx("ns"), "ns", &SubjectVector{
		SubjectID: "u1",
		NumericID: 7,
		Indices:   []uint32{11, 12},
		Values:    []float32{1.5, 2.5},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.CollectionName != "ns_subjects" {
		t.Fatalf("unexpected upsert request: %+v", got)
	}
	if len(got.Points) != 1 {
		t.Fatalf("expected 1 point, got %d", len(got.Points))
	}
	if got.Points[0].Payload["subject_id"].GetStringValue() != "u1" {
		t.Fatalf("unexpected payload: %+v", got.Points[0].Payload)
	}
}

func TestUpsertSubjectVector_UpsertError(t *testing.T) {
	svc := newTestService(&fakeComputeRepo{}, newFakeIDMap())
	svc.upsertFn = func(_ context.Context, _ *qdrant.UpsertPoints) error {
		return context.DeadlineExceeded
	}

	err := svc.upsertSubjectVector(leasedCtx("ns"), "ns", &SubjectVector{SubjectID: "u1", NumericID: 1})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUpsertObjectVectors_UsesExplicitCreatedAt(t *testing.T) {
	svc := newTestService(&fakeComputeRepo{}, newFakeIDMap())
	var got *qdrant.UpsertPoints
	createdAt := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	svc.upsertFn = func(_ context.Context, points *qdrant.UpsertPoints) error {
		got = points
		return nil
	}

	_, err := svc.upsertObjectVectors(leasedCtx("ns"), "ns",
		map[string]map[uint64]float32{"o1": {1: 1.25}},
		map[string]int64{"o1": createdAt.Add(-time.Hour).Unix()},
		map[string]int64{"o1": createdAt.Unix()},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || got.CollectionName != "ns_objects" {
		t.Fatalf("unexpected upsert request: %+v", got)
	}
	payload := got.Points[0].Payload
	if payload["object_id"].GetStringValue() != "o1" {
		t.Fatalf("unexpected object_id payload: %+v", payload)
	}
	if payload["created_at"].GetStringValue() != createdAt.Format(time.RFC3339) {
		t.Fatalf("unexpected created_at: %s", payload["created_at"].GetStringValue())
	}
}

func TestUpsertObjectVectors_UsesMaxOccurredAtFallback(t *testing.T) {
	svc := newTestService(&fakeComputeRepo{}, newFakeIDMap())
	var got *qdrant.UpsertPoints
	maxTime := time.Now().Add(-2 * time.Hour).UTC().Truncate(time.Second)
	svc.upsertFn = func(_ context.Context, points *qdrant.UpsertPoints) error {
		got = points
		return nil
	}

	_, err := svc.upsertObjectVectors(leasedCtx("ns"), "ns",
		map[string]map[uint64]float32{"o1": {1: 1.25}},
		map[string]int64{"o1": maxTime.Unix()},
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Points[0].Payload["created_at"].GetStringValue() != maxTime.Format(time.RFC3339) {
		t.Fatalf("unexpected created_at: %s", got.Points[0].Payload["created_at"].GetStringValue())
	}
}

func TestUpsertObjectVectors_ObjectIDError(t *testing.T) {
	idmap := newFakeIDMap()
	idmap.objectErrs["o1"] = context.DeadlineExceeded
	svc := newTestService(&fakeComputeRepo{}, idmap)

	_, err := svc.upsertObjectVectors(leasedCtx("ns"), "ns",
		map[string]map[uint64]float32{"o1": {1: 1.25}},
		nil,
		nil,
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUpsertObjectVectors_UpsertError(t *testing.T) {
	svc := newTestService(&fakeComputeRepo{}, newFakeIDMap())
	svc.upsertFn = func(_ context.Context, _ *qdrant.UpsertPoints) error {
		return context.DeadlineExceeded
	}

	_, err := svc.upsertObjectVectors(leasedCtx("ns"), "ns",
		map[string]map[uint64]float32{"o1": {1: 1.25}},
		nil,
		nil,
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRecomputeNamespace_ContinuesOnBuildAndUpsertFailures(t *testing.T) {
	now := time.Now().Unix()
	repo := &fakeComputeRepo{
		subjects: []string{"u1", "u2", "u3"},
		subjectEvents: map[string][]*RawEvent{
			"u1": {{SubjectID: "u1", ObjectID: "o1", Weight: 1, OccurredAt: now}},
			"u2": {{SubjectID: "u2", ObjectID: "o2", Weight: 2, OccurredAt: now}},
			"u3": {{SubjectID: "u3", ObjectID: "bad", Weight: 3, OccurredAt: now}},
		},
	}
	idmap := newFakeIDMap()
	idmap.objectErrs["bad"] = context.Canceled
	svc := newTestService(repo, idmap)
	callCount := 0
	svc.upsertFn = func(_ context.Context, points *qdrant.UpsertPoints) error {
		callCount++
		if points.CollectionName == "ns_subjects" && points.Points[0].Payload["subject_id"].GetStringValue() == "u2" {
			return context.DeadlineExceeded
		}
		return nil
	}

	_, _, err := svc.RecomputeNamespace(leasedCtx("ns"), "ns", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount == 0 {
		t.Fatal("expected at least one upsert call")
	}
}

func TestRecomputeNamespace_GetActiveSubjectsError(t *testing.T) {
	svc := newTestService(&fakeComputeRepo{err: context.DeadlineExceeded}, newFakeIDMap())

	_, _, err := svc.RecomputeNamespace(context.Background(), "ns", 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestSVDEmbeddings_ProducesVectors(t *testing.T) {
	now := time.Now().Unix()
	events := []*RawEvent{
		{SubjectID: "u1", ObjectID: "o1", Weight: 1, OccurredAt: now},
		{SubjectID: "u1", ObjectID: "o2", Weight: 1, OccurredAt: now},
		{SubjectID: "u2", ObjectID: "o1", Weight: 1, OccurredAt: now},
		{SubjectID: "u2", ObjectID: "o3", Weight: 1, OccurredAt: now},
	}

	vecs, err := SVDEmbeddings(events, 2, defaultLambda)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vecs) != 3 {
		t.Fatalf("expected 3 vectors, got %d", len(vecs))
	}
	if len(vecs["o1"]) != 2 {
		t.Fatalf("expected vector dim 2, got %d", len(vecs["o1"]))
	}
}

func TestRecomputeNamespace_AllUpsertsFailedReturnsError(t *testing.T) {
	now := time.Now().Unix()
	repo := &fakeComputeRepo{
		subjects: []string{"u1", "u2"},
		subjectEvents: map[string][]*RawEvent{
			"u1": {{SubjectID: "u1", ObjectID: "o1", Weight: 1, OccurredAt: now}},
			"u2": {{SubjectID: "u2", ObjectID: "o2", Weight: 2, OccurredAt: now}},
		},
	}
	svc := newTestService(repo, newFakeIDMap())
	svc.upsertFn = func(_ context.Context, _ *qdrant.UpsertPoints) error {
		return context.DeadlineExceeded
	}

	_, _, err := svc.RecomputeNamespace(leasedCtx("ns"), "ns", 0)
	if err == nil {
		t.Fatal("a run where every upsert failed must not report success")
	}
}

func TestRecomputeNamespace_ObjectUpsertFailureReturnsError(t *testing.T) {
	now := time.Now().Unix()
	repo := &fakeComputeRepo{
		subjects: []string{"u1"},
		subjectEvents: map[string][]*RawEvent{
			"u1": {
				{SubjectID: "u1", ObjectID: "o1", Weight: 1, OccurredAt: now},
				{SubjectID: "u1", ObjectID: "o2", Weight: 1, OccurredAt: now},
			},
		},
	}
	svc := newTestService(repo, newFakeIDMap())
	svc.upsertFn = func(_ context.Context, points *qdrant.UpsertPoints) error {
		if points.CollectionName == "ns_objects" {
			return context.DeadlineExceeded
		}
		return nil
	}

	_, _, err := svc.RecomputeNamespace(leasedCtx("ns"), "ns", 0)
	if err == nil {
		t.Fatal("a failed object-vector upsert must fail the phase")
	}
}

func TestSVDEmbeddings_PadsToEmbeddingDim(t *testing.T) {
	now := time.Now().Unix()
	// 2 subjects × 3 objects → rank min(8, 2) = 2, padded up to dim 8.
	events := []*RawEvent{
		{SubjectID: "u1", ObjectID: "o1", Weight: 1, OccurredAt: now},
		{SubjectID: "u1", ObjectID: "o2", Weight: 1, OccurredAt: now},
		{SubjectID: "u2", ObjectID: "o1", Weight: 1, OccurredAt: now},
		{SubjectID: "u2", ObjectID: "o3", Weight: 1, OccurredAt: now},
	}

	vecs, err := SVDEmbeddings(events, 8, defaultLambda)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for id, v := range vecs {
		if len(v) != 8 {
			t.Fatalf("vector %s: got dim %d, want padded 8", id, len(v))
		}
	}
}

// ─── generation-qualified collections ────────────────────────────────────────

// Every sparse write and cleanup sweep resolves its collection from the
// lifecycle lease the run holds. Hard-coding {ns}_subjects would make a run
// that started before a recreate write into the new incarnation's collections.
func TestRecomputeNamespace_WritesIntoLeaseGenerationCollections(t *testing.T) {
	now := time.Now().Unix()
	repo := &fakeComputeRepo{
		subjects: []string{"u1"},
		events:   []*RawEvent{{SubjectID: "u1", ObjectID: "o1", Weight: 1, OccurredAt: now}},
	}
	svc := newTestService(repo, newFakeIDMap())
	var upserted []string
	svc.upsertFn = func(_ context.Context, points *qdrant.UpsertPoints) error {
		upserted = append(upserted, points.CollectionName)
		return nil
	}
	var swept []string
	svc.cleanupFn = func(_ context.Context, collection string, _ map[uint64]struct{}) (int, error) {
		swept = append(swept, collection)
		return 0, nil
	}

	ctx := nslifecycle.ContextWithLease(context.Background(), "ns", 3, nslifecycle.LockShared)
	if _, _, err := svc.RecomputeNamespace(ctx, "ns", 0.05); err != nil {
		t.Fatalf("RecomputeNamespace: %v", err)
	}
	for _, collection := range upserted {
		if !strings.HasPrefix(collection, "ns_g3_") {
			t.Errorf("upsert into %q, want a ns_g3_* collection", collection)
		}
	}
	for _, collection := range swept {
		if !strings.HasPrefix(collection, "ns_g3_") {
			t.Errorf("cleanup swept %q, want a ns_g3_* collection", collection)
		}
	}
	if len(upserted) == 0 || len(swept) == 0 {
		t.Fatalf("expected both upserts and cleanup sweeps, got upserts=%v sweeps=%v", upserted, swept)
	}
}

// Generation 1 keeps the legacy unqualified names so an upgrade does not
// orphan every existing collection.
func TestRecomputeNamespace_Generation1KeepsLegacyCollectionNames(t *testing.T) {
	now := time.Now().Unix()
	repo := &fakeComputeRepo{
		subjects: []string{"u1"},
		events:   []*RawEvent{{SubjectID: "u1", ObjectID: "o1", Weight: 1, OccurredAt: now}},
	}
	svc := newTestService(repo, newFakeIDMap())
	var upserted []string
	svc.upsertFn = func(_ context.Context, points *qdrant.UpsertPoints) error {
		upserted = append(upserted, points.CollectionName)
		return nil
	}

	ctx := nslifecycle.ContextWithLease(context.Background(), "ns", 1, nslifecycle.LockShared)
	if _, _, err := svc.RecomputeNamespace(ctx, "ns", 0.05); err != nil {
		t.Fatalf("RecomputeNamespace: %v", err)
	}
	for _, collection := range upserted {
		if collection != "ns_subjects" && collection != "ns_objects" {
			t.Errorf("generation 1 wrote into %q, want the legacy names", collection)
		}
	}
}

func TestSparseIndex_RefusesNarrowingInsteadOfColliding(t *testing.T) {
	if _, fits := sparseIndex(maxSparseIndex + 1); fits {
		t.Fatal("id past the uint32 index space must be refused, not truncated")
	}
	got, fits := sparseIndex(maxSparseIndex)
	if !fits || got != maxSparseIndex {
		t.Fatalf("boundary id: got %d fits %v", got, fits)
	}
}

// An id past the uint32 index space skips that dimension only: the dimension
// is equally unrepresentable in every vector, so dropping it cannot collide or
// corrupt, while erroring here failed the whole subject (this site) or the
// whole run, permanently, at the object site — the offending id never goes
// away.
func TestBuildSubjectVector_SkipsObjectPastSparseIndexSpace(t *testing.T) {
	idmap := newFakeIDMap()
	idmap.objectIDs["o-bad"] = maxSparseIndex + 1
	idmap.objectIDs["o-good"] = 7
	svc := newTestService(&fakeComputeRepo{}, idmap)

	vec, err := svc.buildSubjectVector(context.Background(), "ns", "u1",
		map[string]float64{"o-bad": 1, "o-good": 2},
		map[string]uint64{"o-bad": maxSparseIndex + 1, "o-good": 7})
	if err != nil {
		t.Fatalf("unrepresentable dimension must not fail the subject: %v", err)
	}
	if len(vec.Indices) != 1 || vec.Indices[0] != 7 {
		t.Fatalf("vector must keep only the representable dimension, got %v", vec.Indices)
	}
}

// Partial truncation degrades; total truncation must not pass as health. A
// subject that had interactions but produced no representable dimension would
// otherwise be upserted as an empty vector and counted toward upserted++, so
// sparse search returns nothing, requests fall to fallback_popular, and the
// run still reports success.
func TestBuildSubjectVector_TotalTruncationIsAnError(t *testing.T) {
	idmap := newFakeIDMap()
	idmap.objectIDs["o-bad"] = maxSparseIndex + 1
	idmap.objectIDs["o-worse"] = maxSparseIndex + 2
	svc := newTestService(&fakeComputeRepo{}, idmap)

	_, err := svc.buildSubjectVector(context.Background(), "ns", "u1",
		map[string]float64{"o-bad": 1, "o-worse": 2},
		map[string]uint64{"o-bad": maxSparseIndex + 1, "o-worse": maxSparseIndex + 2})
	if err == nil {
		t.Fatal("a subject whose every dimension was skipped must fail, not upsert empty")
	}
}

// A subject with no interactions legitimately has an empty vector — that path
// must stay distinct from total truncation.
func TestBuildSubjectVector_NoScoresIsNotTruncation(t *testing.T) {
	svc := newTestService(&fakeComputeRepo{}, newFakeIDMap())

	vec, err := svc.buildSubjectVector(context.Background(), "ns", "u1", map[string]float64{}, map[string]uint64{})
	if err != nil {
		t.Fatalf("empty score set is not truncation: %v", err)
	}
	if len(vec.Indices) != 0 {
		t.Fatalf("expected empty vector, got %v", vec.Indices)
	}
}

func TestUpsertObjectVectors_SkipsCooccurrenceEntryPastSparseIndexSpace(t *testing.T) {
	svc := newTestService(&fakeComputeRepo{}, newFakeIDMap())
	var got *qdrant.UpsertPoints
	svc.upsertFn = func(_ context.Context, points *qdrant.UpsertPoints) error {
		got = points
		return nil
	}
	accum := map[string]map[uint64]float32{"o1": {maxSparseIndex + 1: 1, 5: 2}}

	if _, err := svc.upsertObjectVectors(leasedCtx("ns"), "ns", accum, nil, nil); err != nil {
		t.Fatalf("unrepresentable dimension must not fail the run: %v", err)
	}
	idx := got.Points[0].GetVectors().GetVectors().GetVectors()[sparseVectorName].GetSparse().GetIndices()
	if len(idx) != 1 || idx[0] != 5 {
		t.Fatalf("row must keep only the representable dimension, got %v", idx)
	}
}

func TestL2Normalize(t *testing.T) {
	v := []float32{3, 4}
	l2Normalize(v)
	if math.Abs(float64(v[0])-0.6) > 1e-6 || math.Abs(float64(v[1])-0.8) > 1e-6 {
		t.Fatalf("normalized = %v, want [0.6 0.8]", v)
	}
	zero := []float32{0, 0}
	l2Normalize(zero)
	if zero[0] != 0 || zero[1] != 0 {
		t.Fatalf("zero vector must stay zero, got %v", zero)
	}
}

// The stored vectors are what makes sparse dot products cosines; if either
// side ships unnormalized the serving clamp is wrong, so pin both.
func TestBuildSubjectVector_IsUnitNorm(t *testing.T) {
	svc := newTestService(&fakeComputeRepo{}, newFakeIDMap())
	vec, err := svc.buildSubjectVector(context.Background(), "ns", "u1",
		map[string]float64{"o1": 3, "o2": 4}, map[string]uint64{"o1": 11, "o2": 12})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var sum float64
	for _, v := range vec.Values {
		sum += float64(v) * float64(v)
	}
	if math.Abs(sum-1) > 1e-6 {
		t.Fatalf("subject vector norm² = %f, want 1", sum)
	}
}

func TestUpsertObjectVectors_RowsAreUnitNorm(t *testing.T) {
	svc := newTestService(&fakeComputeRepo{}, newFakeIDMap())
	var got *qdrant.UpsertPoints
	svc.upsertFn = func(_ context.Context, points *qdrant.UpsertPoints) error {
		got = points
		return nil
	}
	accum := map[string]map[uint64]float32{"o1": {1: 3, 2: 4}}
	if _, err := svc.upsertObjectVectors(leasedCtx("ns"), "ns", accum, nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	vals := got.Points[0].GetVectors().GetVectors().GetVectors()[sparseVectorName].GetSparse().GetValues()
	var sum float64
	for _, v := range vals {
		sum += float64(v) * float64(v)
	}
	if math.Abs(sum-1) > 1e-6 {
		t.Fatalf("object row norm² = %f, want 1", sum)
	}
}

// Object ids must be resolved once per subject through the batch API, not one
// sequential query per object per consumer. The old shape cost 2N round-trips
// per subject per tick — buildSubjectVector and accumulateObjectCooccurrence
// each resolving the same key set — which measured as ~3.48M resolutions
// against 43k real rows in under two hours on the development stack.
func TestRecomputeNamespace_ResolvesObjectIDsInBatches(t *testing.T) {
	now := time.Now().Unix()
	repo := &fakeComputeRepo{
		subjects: []string{"u1", "u2"},
		subjectEvents: map[string][]*RawEvent{
			"u1": {
				{SubjectID: "u1", ObjectID: "o1", Weight: 1, OccurredAt: now},
				{SubjectID: "u1", ObjectID: "o2", Weight: 1, OccurredAt: now},
				{SubjectID: "u1", ObjectID: "o3", Weight: 1, OccurredAt: now},
			},
			"u2": {
				{SubjectID: "u2", ObjectID: "o2", Weight: 1, OccurredAt: now},
				{SubjectID: "u2", ObjectID: "o4", Weight: 1, OccurredAt: now},
			},
		},
	}
	idmap := newFakeIDMap()
	svc := newTestService(repo, idmap)

	if _, _, err := svc.RecomputeNamespace(leasedCtx("ns"), "ns", 0.05); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if idmap.singleCalls != 0 {
		t.Fatalf("object ids must resolve in batches, got %d per-id calls", idmap.singleCalls)
	}
	// One batch per subject plus one for the accumulated object rows.
	if idmap.batchCalls != 3 {
		t.Fatalf("expected 3 batch resolutions (2 subjects + object rows), got %d", idmap.batchCalls)
	}
}
