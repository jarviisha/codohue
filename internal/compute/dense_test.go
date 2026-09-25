package compute

import (
	"context"
	"errors"
	"math"
	"strconv"
	"testing"

	"github.com/jarviisha/codohue/internal/core/idmap"
	"github.com/qdrant/go-client/qdrant"
)

// ─────────────────────────────────────────────────────────────
// BuildInteractionSequences
// ─────────────────────────────────────────────────────────────

func TestBuildInteractionSequencesBasic(t *testing.T) {
	t.Parallel()
	events := []*RawEvent{
		{SubjectID: "u1", ObjectID: "p1", OccurredAt: 100},
		{SubjectID: "u1", ObjectID: "p2", OccurredAt: 200},
		{SubjectID: "u2", ObjectID: "p3", OccurredAt: 150},
	}
	seqs := BuildInteractionSequences(events)

	if len(seqs) != 2 {
		t.Fatalf("expected 2 sequences, got %d", len(seqs))
	}
	if seqs[0].SubjectID != "u1" || len(seqs[0].ObjectIDs) != 2 {
		t.Errorf("u1 sequence wrong: %+v", seqs[0])
	}
	if seqs[1].SubjectID != "u2" || len(seqs[1].ObjectIDs) != 1 {
		t.Errorf("u2 sequence wrong: %+v", seqs[1])
	}
	// Items within u1 sequence should be in insertion order (p1 before p2).
	if seqs[0].ObjectIDs[0] != "p1" || seqs[0].ObjectIDs[1] != "p2" {
		t.Errorf("u1 item order wrong: %v", seqs[0].ObjectIDs)
	}
}

func TestBuildInteractionSequencesEmpty(t *testing.T) {
	t.Parallel()
	seqs := BuildInteractionSequences(nil)
	if len(seqs) != 0 {
		t.Errorf("expected empty result for nil input, got %d", len(seqs))
	}
}

// ─────────────────────────────────────────────────────────────
// TrainItem2Vec
// ─────────────────────────────────────────────────────────────

func TestTrainItem2VecProducesVectors(t *testing.T) {
	t.Parallel()
	// Build enough sequences to exceed min_count=2.
	seqs := []InteractionSequence{
		{SubjectID: "u1", ObjectIDs: []string{"A", "B", "C", "D"}},
		{SubjectID: "u2", ObjectIDs: []string{"B", "C", "D", "E"}},
		{SubjectID: "u3", ObjectIDs: []string{"A", "C", "D", "E"}},
	}
	cfg := Item2VecConfig{Dim: 8, Window: 2, MinCount: 2, Epochs: 3, NegSamples: 2}

	result := TrainItem2Vec(seqs, cfg)

	if len(result) == 0 {
		t.Fatal("expected non-empty embedding map")
	}
	for item, vec := range result {
		if len(vec) != cfg.Dim {
			t.Errorf("item %q: vector length = %d, want %d", item, len(vec), cfg.Dim)
		}
		// Vectors should not be all-zero after training.
		allZero := true
		for _, v := range vec {
			if v != 0 {
				allZero = false
				break
			}
		}
		if allZero {
			t.Errorf("item %q: all-zero vector after training", item)
		}
	}
}

func TestTrainItem2VecMinCountFiltering(t *testing.T) {
	t.Parallel()
	seqs := []InteractionSequence{
		{SubjectID: "u1", ObjectIDs: []string{"common", "common", "rare"}},
		{SubjectID: "u2", ObjectIDs: []string{"common", "common"}},
	}
	cfg := Item2VecConfig{Dim: 4, Window: 1, MinCount: 3, Epochs: 1, NegSamples: 1}

	result := TrainItem2Vec(seqs, cfg)

	if _, ok := result["rare"]; ok {
		t.Error("item 'rare' (count=1) should be filtered by min_count=3")
	}
}

func TestTrainItem2VecTooSmallVocab(t *testing.T) {
	t.Parallel()
	seqs := []InteractionSequence{
		{SubjectID: "u1", ObjectIDs: []string{"only_item"}},
	}
	cfg := Item2VecConfig{Dim: 4, Window: 1, MinCount: 1, Epochs: 1, NegSamples: 1}
	result := TrainItem2Vec(seqs, cfg)
	if len(result) > 0 {
		t.Error("vocab of size 1 should produce no output (need at least 2 items)")
	}
}

func TestTrainItem2VecEmptyInput(t *testing.T) {
	t.Parallel()
	cfg := Item2VecConfig{Dim: 4, Window: 1, MinCount: 1, Epochs: 1, NegSamples: 1}
	if result := TrainItem2Vec(nil, cfg); len(result) != 0 {
		t.Fatalf("expected empty result, got %v", result)
	}
}

// ─────────────────────────────────────────────────────────────
// UserDenseVectors
// ─────────────────────────────────────────────────────────────

func TestUserDenseVectors(t *testing.T) {
	t.Parallel()
	itemVecs := map[string][]float32{
		"p1": {1.0, 0.0},
		"p2": {0.0, 1.0},
		"p3": {1.0, 1.0},
	}
	events := []*RawEvent{
		{SubjectID: "u1", ObjectID: "p1"},
		{SubjectID: "u1", ObjectID: "p2"},
		{SubjectID: "u2", ObjectID: "p3"},
		{SubjectID: "u3", ObjectID: "unknown_item"}, // no dense vector — skipped
	}

	result := UserDenseVectors(events, itemVecs)

	if len(result) != 2 {
		t.Fatalf("expected 2 user vectors (u1, u2), got %d", len(result))
	}

	// u1 mean([1,0], [0,1]) = [0.5, 0.5]
	u1 := result["u1"]
	if len(u1) != 2 {
		t.Fatalf("u1 vector length = %d, want 2", len(u1))
	}
	if math.Abs(float64(u1[0]-0.5)) > 1e-6 || math.Abs(float64(u1[1]-0.5)) > 1e-6 {
		t.Errorf("u1 mean vector = %v, want [0.5, 0.5]", u1)
	}

	// u2 mean([1,1]) = [1.0, 1.0]
	u2 := result["u2"]
	if math.Abs(float64(u2[0]-1.0)) > 1e-6 || math.Abs(float64(u2[1]-1.0)) > 1e-6 {
		t.Errorf("u2 mean vector = %v, want [1.0, 1.0]", u2)
	}

	// u3 has no item vectors — should not appear in result.
	if _, ok := result["u3"]; ok {
		t.Error("u3 has no dense item interactions, should be absent from result")
	}
}

// ─────────────────────────────────────────────────────────────
// SGD and math helpers
// ─────────────────────────────────────────────────────────────

func TestSGDUpdatePositivePairIncreasesAlignment(t *testing.T) {
	t.Parallel()
	// After a positive update, dot(target, output) should increase.
	target := []float32{1.0, 0.0, 0.0}
	output := []float32{0.0, 1.0, 0.0}

	dotBefore := float32(0)
	for d := range target {
		dotBefore += target[d] * output[d]
	}

	sgdUpdate(target, output, 1.0, 0.1)

	dotAfter := float32(0)
	for d := range target {
		dotAfter += target[d] * output[d]
	}
	if dotAfter <= dotBefore {
		t.Errorf("positive update should increase dot product: before=%f, after=%f", dotBefore, dotAfter)
	}
}

func TestSGDUpdateNegativePairDecreasesAlignment(t *testing.T) {
	t.Parallel()
	// Start with a positive dot product; a negative update should reduce it.
	target := []float32{1.0, 0.5, 0.0}
	output := []float32{1.0, 0.5, 0.0}

	dotBefore := float32(0)
	for d := range target {
		dotBefore += target[d] * output[d]
	}

	sgdUpdate(target, output, 0.0, 0.1)

	dotAfter := float32(0)
	for d := range target {
		dotAfter += target[d] * output[d]
	}
	if dotAfter >= dotBefore {
		t.Errorf("negative update should decrease dot product: before=%f, after=%f", dotBefore, dotAfter)
	}
}

func TestSigmoid32Bounds(t *testing.T) {
	t.Parallel()
	tests := []struct{ x, lo, hi float32 }{
		{0, 0.49, 0.51},
		{100, 0.999, 1.001},
		{-100, -0.001, 0.001},
	}
	for _, tt := range tests {
		got := sigmoid32(tt.x)
		if got < tt.lo || got > tt.hi {
			t.Errorf("sigmoid32(%f) = %f, want [%f, %f]", tt.x, got, tt.lo, tt.hi)
		}
	}
}

type fakeDenseIDRepo struct {
	ids  map[string]uint64
	errs map[string]error
	next uint64
}

func (f *fakeDenseIDRepo) GetOrCreate(_ context.Context, stringID, _, _ string) (uint64, error) {
	if err, ok := f.errs[stringID]; ok {
		return 0, err
	}
	if id, ok := f.ids[stringID]; ok {
		return id, nil
	}
	f.next++
	f.ids[stringID] = f.next
	return f.next, nil
}

func (f *fakeDenseIDRepo) Lookup(_ context.Context, stringID, _, _ string) (numericID uint64, found bool, err error) {
	id, ok := f.ids[stringID]
	return id, ok, nil
}

func (f *fakeDenseIDRepo) GetOrCreateBatch(ctx context.Context, stringIDs []string, ns, entityType string) (map[string]uint64, error) {
	out := make(map[string]uint64, len(stringIDs))
	for _, id := range stringIDs {
		numID, err := f.GetOrCreate(ctx, id, ns, entityType)
		if err != nil {
			continue
		}
		out[id] = numID
	}
	return out, nil
}

// fakeDenseClient implements densePointClient; a nil field fails the test if
// the operation is reached.
type fakeDenseClient struct {
	t      *testing.T
	upsert func(points *qdrant.UpsertPoints) (*qdrant.UpdateResult, error)
	get    func(req *qdrant.GetPoints) ([]*qdrant.RetrievedPoint, error)
}

func (f *fakeDenseClient) Upsert(_ context.Context, points *qdrant.UpsertPoints) (*qdrant.UpdateResult, error) {
	if f.upsert == nil {
		f.t.Fatal("unexpected qdrant upsert")
	}
	return f.upsert(points)
}

func (f *fakeDenseClient) Get(_ context.Context, req *qdrant.GetPoints) ([]*qdrant.RetrievedPoint, error) {
	if f.get == nil {
		f.t.Fatal("unexpected qdrant get")
	}
	return f.get(req)
}

func TestUpsertItemDenseVectors_Success(t *testing.T) {
	t.Parallel()
	repo := &fakeDenseIDRepo{ids: map[string]uint64{}, errs: map[string]error{}, next: 10}
	idmapSvc := idmap.NewService(repo)
	called := false
	client := &fakeDenseClient{t: t, upsert: func(points *qdrant.UpsertPoints) (*qdrant.UpdateResult, error) {
		called = true
		if points.CollectionName != "ns_objects_dense" {
			t.Fatalf("unexpected collection: %s", points.CollectionName)
		}
		if len(points.Points) != 2 {
			t.Fatalf("expected 2 points, got %d", len(points.Points))
		}
		return nil, nil
	}}

	err := UpsertItemDenseVectors(leasedCtx("ns"), client, idmapSvc, "ns", "item2vec", map[string][]float32{
		"obj-1": {0.1, 0.2},
		"obj-2": {0.3, 0.4},
	}, map[string]string{"obj-1": "2026-07-01T00:00:00Z"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected qdrant upsert to be called")
	}
}

func TestUpsertSubjectDenseVectors_SkipsIDMappingErrors(t *testing.T) {
	t.Parallel()
	repo := &fakeDenseIDRepo{
		ids:  map[string]uint64{"sub-1": 1},
		errs: map[string]error{"sub-bad": errors.New("mapping failed")},
	}
	idmapSvc := idmap.NewService(repo)
	client := &fakeDenseClient{t: t, upsert: func(points *qdrant.UpsertPoints) (*qdrant.UpdateResult, error) {
		if len(points.Points) != 1 {
			t.Fatalf("expected 1 point after skipping bad id, got %d", len(points.Points))
		}
		return nil, nil
	}}

	err := UpsertSubjectDenseVectors(leasedCtx("ns"), client, idmapSvc, "ns", "item2vec", map[string][]float32{
		"sub-1":   {0.1, 0.2},
		"sub-bad": {0.3, 0.4},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpsertDenseVectors_UpsertError(t *testing.T) {
	t.Parallel()
	repo := &fakeDenseIDRepo{ids: map[string]uint64{"obj-1": 1}, errs: map[string]error{}}
	idmapSvc := idmap.NewService(repo)
	client := &fakeDenseClient{t: t, upsert: func(_ *qdrant.UpsertPoints) (*qdrant.UpdateResult, error) {
		return nil, errors.New("upsert failed")
	}}

	err := upsertDenseVectors(context.Background(), client, idmapSvc, "ns_objects_dense", "ns", "object", "item2vec", map[string][]float32{
		"obj-1": {0.1, 0.2},
	}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// densePoint builds a RetrievedPoint shaped like what both cmd/embedder and
// upsertDenseVectors write, so the fetch path is tested against the real
// named-vector layout.
func densePoint(numID uint64, vec []float32) *qdrant.RetrievedPoint {
	return &qdrant.RetrievedPoint{
		Id: qdrant.NewIDNum(numID),
		Vectors: &qdrant.VectorsOutput{
			VectorsOptions: &qdrant.VectorsOutput_Vectors{
				Vectors: &qdrant.NamedVectorsOutput{
					Vectors: map[string]*qdrant.VectorOutput{
						denseVectorName: {
							Vector: &qdrant.VectorOutput_Dense{
								Dense: &qdrant.DenseVector{Data: vec},
							},
						},
					},
				},
			},
		},
	}
}

func TestFetchItemDenseVectors_Success(t *testing.T) {
	t.Parallel()
	repo := &fakeDenseIDRepo{ids: map[string]uint64{"obj-1": 7, "obj-2": 8}, errs: map[string]error{}}
	idmapSvc := idmap.NewService(repo)
	client := &fakeDenseClient{t: t, get: func(req *qdrant.GetPoints) ([]*qdrant.RetrievedPoint, error) {
		if req.CollectionName != "ns_objects_dense" {
			t.Fatalf("unexpected collection: %s", req.CollectionName)
		}
		if len(req.Ids) != 2 {
			t.Fatalf("expected 2 ids requested, got %d", len(req.Ids))
		}
		return []*qdrant.RetrievedPoint{
			densePoint(7, []float32{0.1, 0.2}),
			densePoint(8, []float32{0.3, 0.4}),
		}, nil
	}}

	got, err := FetchItemDenseVectors(leasedCtx("ns"), client, idmapSvc, "ns", []string{"obj-2", "obj-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 vectors, got %d", len(got))
	}
	if v := got["obj-1"]; len(v) != 2 || v[0] != 0.1 {
		t.Errorf("obj-1 vector = %v", v)
	}
	if v := got["obj-2"]; len(v) != 2 || v[0] != 0.3 {
		t.Errorf("obj-2 vector = %v", v)
	}
}

// Objects the embedder has not written yet come back as missing points; they
// must be absent from the result rather than an error.
func TestFetchItemDenseVectors_MissingPointsOmitted(t *testing.T) {
	t.Parallel()
	repo := &fakeDenseIDRepo{ids: map[string]uint64{"obj-1": 7, "obj-2": 8}, errs: map[string]error{}}
	idmapSvc := idmap.NewService(repo)
	client := &fakeDenseClient{t: t, get: func(_ *qdrant.GetPoints) ([]*qdrant.RetrievedPoint, error) {
		return []*qdrant.RetrievedPoint{densePoint(7, []float32{0.1, 0.2})}, nil
	}}

	got, err := FetchItemDenseVectors(leasedCtx("ns"), client, idmapSvc, "ns", []string{"obj-1", "obj-2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected only the embedded object, got %v", got)
	}
	if _, ok := got["obj-2"]; ok {
		t.Error("obj-2 has no point and must be omitted")
	}
}

func TestFetchItemDenseVectors_GetError(t *testing.T) {
	t.Parallel()
	repo := &fakeDenseIDRepo{ids: map[string]uint64{"obj-1": 7}, errs: map[string]error{}}
	idmapSvc := idmap.NewService(repo)
	client := &fakeDenseClient{t: t, get: func(_ *qdrant.GetPoints) ([]*qdrant.RetrievedPoint, error) {
		return nil, errors.New("qdrant down")
	}}

	if _, err := FetchItemDenseVectors(leasedCtx("ns"), client, idmapSvc, "ns", []string{"obj-1"}); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFetchItemDenseVectors_EmptyIsNoOp(t *testing.T) {
	t.Parallel()
	// A nil get fn fails the test if the fetch reaches Qdrant at all.
	client := &fakeDenseClient{t: t}

	got, err := FetchItemDenseVectors(leasedCtx("ns"), client, idmap.NewService(
		&fakeDenseIDRepo{ids: map[string]uint64{}, errs: map[string]error{}}), "ns", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
}

func TestUpsertDenseVectors_EmptyIsNoOp(t *testing.T) {
	t.Parallel()
	repo := &fakeDenseIDRepo{ids: map[string]uint64{}, errs: map[string]error{}}
	idmapSvc := idmap.NewService(repo)
	// A nil upsert fn fails the test if the empty upsert reaches Qdrant.
	client := &fakeDenseClient{t: t}

	if err := upsertDenseVectors(context.Background(), client, idmapSvc, "ns_objects_dense", "ns", "object", "item2vec", nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSVDEmbeddings_EmptyReturnsNil(t *testing.T) {
	t.Parallel()
	vecs, err := SVDEmbeddings(nil, 4, defaultLambda)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vecs != nil {
		t.Fatalf("expected nil vectors, got %v", vecs)
	}
}

func TestSVDEmbeddings_MatrixTooLargeReturnsError(t *testing.T) {
	t.Parallel()
	events := make([]*RawEvent, 0, 10001)
	for i := 0; i < 10001; i++ {
		events = append(events, &RawEvent{
			SubjectID:  "subject-" + strconv.Itoa(i),
			ObjectID:   "object-" + strconv.Itoa(i%1001),
			Weight:     1,
			OccurredAt: 1,
		})
	}

	_, err := SVDEmbeddings(events, 8, defaultLambda)
	if err == nil {
		t.Fatal("expected matrix too large error, got nil")
	}
}
