package compute

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/qdrant/go-client/qdrant"
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
	subjectID  uint64
	subjectErr error
	objectIDs  map[string]uint64
	nextID     uint64
	objectErrs map[string]error
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

func (f *fakeIDMap) GetOrCreateObjectID(_ context.Context, objectID, _ string) (uint64, error) {
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

	_, scores, maxTimes, _, err := svc.buildVectors(context.Background(), "ns", "u1", 0.05)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Event happened now → freshness ≈ 1.0, score ≈ weight.
	if math.Abs(scores["o1"]-5.0) > 0.01 {
		t.Errorf("score: got %.4f, want ≈5.0", scores["o1"])
	}
	if maxTimes["o1"] != now {
		t.Errorf("maxTime: got %d, want %d", maxTimes["o1"], now)
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
	_, scores, _, _, err := svc.buildVectors(context.Background(), "ns", "u1", lambda)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := math.Exp(-lambda * 10)
	if math.Abs(scores["o1"]-want) > 0.01 {
		t.Errorf("decayed score: got %.6f, want %.6f", scores["o1"], want)
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

	_, scores, _, _, err := svc.buildVectors(context.Background(), "ns", "u1", 0.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// lambda=0 → freshness=1.0 everywhere, scores are pure sums.
	if math.Abs(scores["o1"]-5.0) > 1e-9 {
		t.Errorf("o1 accumulated score: got %.4f, want 5.0", scores["o1"])
	}
	if math.Abs(scores["o2"]-1.0) > 1e-9 {
		t.Errorf("o2 score: got %.4f, want 1.0", scores["o2"])
	}
}

func TestBuildVectors_ObjectCreatedAtTracked(t *testing.T) {
	now := time.Now().Unix()
	created := now - 1000
	events := []*RawEvent{
		{SubjectID: "u1", ObjectID: "o1", Weight: 1.0, OccurredAt: now, ObjectCreatedAt: &created},
	}
	svc := newTestService(&fakeComputeRepo{events: events}, newFakeIDMap())

	_, _, _, createdTimes, err := svc.buildVectors(context.Background(), "ns", "u1", 0.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if createdTimes["o1"] != created {
		t.Errorf("createdTime: got %d, want %d", createdTimes["o1"], created)
	}
}

func TestBuildVectors_NoEvents_EmptyResult(t *testing.T) {
	svc := newTestService(&fakeComputeRepo{events: nil}, newFakeIDMap())

	vec, scores, maxTimes, createdTimes, err := svc.buildVectors(context.Background(), "ns", "u1", 0.05)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(scores) != 0 {
		t.Errorf("expected empty scores, got %v", scores)
	}
	if len(maxTimes) != 0 {
		t.Errorf("expected empty maxTimes, got %v", maxTimes)
	}
	if len(createdTimes) != 0 {
		t.Errorf("expected empty createdTimes, got %v", createdTimes)
	}
	if vec == nil {
		t.Fatal("expected non-nil SubjectVector even for empty events")
	}
	if len(vec.Indices) != 0 {
		t.Errorf("expected empty indices, got %v", vec.Indices)
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

	_, _, maxTimes, _, err := svc.buildVectors(context.Background(), "ns", "u1", 0.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if maxTimes["o1"] != newer {
		t.Errorf("maxTime: got %d, want %d (newer)", maxTimes["o1"], newer)
	}
}

// ─── existing accumulation + decay helpers (kept for regression) ──────────────

func TestObjectAccumulation(t *testing.T) {
	type subjectResult struct {
		numericID uint64
		scores    map[string]float64
	}

	subjects := []subjectResult{
		{numericID: 1, scores: map[string]float64{"obj-A": 2.0, "obj-B": 1.5}},
		{numericID: 2, scores: map[string]float64{"obj-A": 0.8, "obj-C": 3.0}},
	}

	objectAccum := make(map[string]map[uint64]float32)
	for _, s := range subjects {
		for objID, score := range s.scores {
			if objectAccum[objID] == nil {
				objectAccum[objID] = make(map[uint64]float32)
			}
			objectAccum[objID][s.numericID] = float32(score)
		}
	}

	if got := objectAccum["obj-A"][1]; math.Abs(float64(got)-2.0) > 1e-6 {
		t.Errorf("obj-A[subj1] = %v, want 2.0", got)
	}
	if got := objectAccum["obj-A"][2]; math.Abs(float64(got)-0.8) > 1e-6 {
		t.Errorf("obj-A[subj2] = %v, want 0.8", got)
	}
	if _, exists := objectAccum["obj-B"][2]; exists {
		t.Error("obj-B should not have score for subject 2")
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

	_, err := svc.buildSubjectVector(context.Background(), "ns", "u1", map[string]float64{"o1": 1})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestBuildSubjectVector_ObjectIDError(t *testing.T) {
	idmap := newFakeIDMap()
	idmap.objectErrs["o1"] = context.Canceled
	svc := newTestService(&fakeComputeRepo{}, idmap)

	_, err := svc.buildSubjectVector(context.Background(), "ns", "u1", map[string]float64{"o1": 1})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUpsertSubjectVector_SendsExpectedPayload(t *testing.T) {
	svc := newTestService(&fakeComputeRepo{}, newFakeIDMap())
	var got *qdrant.UpsertPoints
	svc.upsertFn = func(_ context.Context, points *qdrant.UpsertPoints) error {
		got = points
		return nil
	}

	err := svc.upsertSubjectVector(context.Background(), "ns", &SubjectVector{
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

	err := svc.upsertSubjectVector(context.Background(), "ns", &SubjectVector{SubjectID: "u1", NumericID: 1})
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

	err := svc.upsertObjectVectors(context.Background(), "ns",
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

	err := svc.upsertObjectVectors(context.Background(), "ns",
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

	err := svc.upsertObjectVectors(context.Background(), "ns",
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

	err := svc.upsertObjectVectors(context.Background(), "ns",
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

	err := svc.RecomputeNamespace(context.Background(), "ns", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount == 0 {
		t.Fatal("expected at least one upsert call")
	}
}

func TestRecomputeNamespace_GetActiveSubjectsError(t *testing.T) {
	svc := newTestService(&fakeComputeRepo{err: context.DeadlineExceeded}, newFakeIDMap())

	err := svc.RecomputeNamespace(context.Background(), "ns", 0)
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

	vecs, err := SVDEmbeddings(events, 2)
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
