package qdrant

import (
	"context"
	"errors"
	"testing"

	"github.com/qdrant/go-client/qdrant"
)

// denseVectorsOutput builds the named-vector shape Qdrant returns for a dense
// point, which is what the repair primitives have to preserve exactly.
func denseVectorsOutput(values ...float32) *qdrant.VectorsOutput {
	return &qdrant.VectorsOutput{
		VectorsOptions: &qdrant.VectorsOutput_Vectors{
			Vectors: &qdrant.NamedVectorsOutput{
				Vectors: map[string]*qdrant.VectorOutput{
					"dense": {Vector: &qdrant.VectorOutput_Dense{Dense: &qdrant.DenseVector{Data: values}}},
				},
			},
		},
	}
}

// fakePointStore is an in-memory stand-in for one Qdrant collection.
type fakePointStore struct {
	points    map[uint64]*qdrant.RetrievedPoint
	pageSize  int
	order     []uint64
	scrollErr error
	upsertErr error
	deleteErr error
	getErr    error
	// corruptOnUpsert mutates what lands, so verification has something real
	// to catch.
	corruptOnUpsert bool
}

func newFakePointStore(pageSize int) *fakePointStore {
	return &fakePointStore{points: map[uint64]*qdrant.RetrievedPoint{}, pageSize: pageSize}
}

func (f *fakePointStore) add(id uint64, stringIDField, stringID string, vector []float32) {
	f.points[id] = &qdrant.RetrievedPoint{
		Id:      qdrant.NewIDNum(id),
		Payload: map[string]*qdrant.Value{stringIDField: qdrant.NewValueString(stringID)},
		Vectors: denseVectorsOutput(vector...),
	}
	f.order = append(f.order, id)
}

func (f *fakePointStore) ScrollAndOffset(_ context.Context, req *qdrant.ScrollPoints) ([]*qdrant.RetrievedPoint, *qdrant.PointId, error) {
	if f.scrollErr != nil {
		return nil, nil, f.scrollErr
	}
	start := 0
	if req.Offset != nil {
		for i, id := range f.order {
			if id == req.Offset.GetNum() {
				start = i
				break
			}
		}
	}
	end := start + f.pageSize
	var next *qdrant.PointId
	if end >= len(f.order) {
		end = len(f.order)
	} else {
		next = qdrant.NewIDNum(f.order[end])
	}
	out := make([]*qdrant.RetrievedPoint, 0, end-start)
	for _, id := range f.order[start:end] {
		out = append(out, f.points[id])
	}
	return out, next, nil
}

func (f *fakePointStore) Upsert(_ context.Context, req *qdrant.UpsertPoints) (*qdrant.UpdateResult, error) {
	if f.upsertErr != nil {
		return nil, f.upsertErr
	}
	for _, point := range req.Points {
		id := point.GetId().GetNum()
		vectors := denseVectorsOutput(namedDense(point.GetVectors())...)
		if f.corruptOnUpsert {
			vectors = denseVectorsOutput(0, 0)
		}
		f.points[id] = &qdrant.RetrievedPoint{Id: point.GetId(), Payload: point.GetPayload(), Vectors: vectors}
		f.order = append(f.order, id)
	}
	return &qdrant.UpdateResult{}, nil
}

func (f *fakePointStore) Delete(_ context.Context, req *qdrant.DeletePoints) (*qdrant.UpdateResult, error) {
	if f.deleteErr != nil {
		return nil, f.deleteErr
	}
	for _, id := range req.GetPoints().GetPoints().GetIds() {
		delete(f.points, id.GetNum())
	}
	return &qdrant.UpdateResult{}, nil
}

func (f *fakePointStore) Get(_ context.Context, req *qdrant.GetPoints) ([]*qdrant.RetrievedPoint, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	var out []*qdrant.RetrievedPoint
	for _, id := range req.Ids {
		if point, ok := f.points[id.GetNum()]; ok {
			out = append(out, point)
		}
	}
	return out, nil
}

// The inventory is the audit's evidence, so it must cover every point across
// pages — a truncated walk would silently exclude identities from the manifest
// and leave them behind.
func TestInventoryCollection_WalksEveryPage(t *testing.T) {
	store := newFakePointStore(2)
	for i := uint64(1); i <= 5; i++ {
		store.add(i, "object_id", "o"+string(rune('0'+i)), []float32{float32(i), 0})
	}

	got, err := InventoryCollection(context.Background(), store, "ns_objects_dense", "object_id")
	if err != nil {
		t.Fatalf("InventoryCollection: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("inventoried %d points, want 5", len(got))
	}
	for _, point := range got {
		if point.StringID == "" || point.VectorHash == "" {
			t.Errorf("incomplete inventory entry: %+v", point)
		}
	}
}

// A point with no logical id in its payload is exactly the ambiguous evidence
// the audit must quarantine. Dropping it here would hide it.
func TestInventoryCollection_KeepsUnlabeledPoints(t *testing.T) {
	store := newFakePointStore(10)
	store.points[7] = &qdrant.RetrievedPoint{
		Id:      qdrant.NewIDNum(7),
		Payload: map[string]*qdrant.Value{},
		Vectors: denseVectorsOutput(1, 2),
	}
	store.order = append(store.order, 7)

	got, err := InventoryCollection(context.Background(), store, "ns_objects_dense", "object_id")
	if err != nil {
		t.Fatalf("InventoryCollection: %v", err)
	}
	if len(got) != 1 || got[0].StringID != "" {
		t.Fatalf("an unlabeled point must be reported, got %+v", got)
	}
}

func TestInventoryCollection_ScrollErrorPropagates(t *testing.T) {
	store := newFakePointStore(10)
	store.scrollErr = errors.New("qdrant down")

	if _, err := InventoryCollection(context.Background(), store, "ns_objects", "object_id"); err == nil {
		t.Fatal("expected the scroll error to surface")
	}
}

// Go map iteration order is random, so an unsorted payload hash would report
// identical payloads as different and fail every verified copy.
func TestHashPayload_IsStableAcrossKeyOrder(t *testing.T) {
	a := map[string]*qdrant.Value{
		"object_id":  qdrant.NewValueString("o1"),
		"created_at": qdrant.NewValueInt(1700000000),
		"strategy":   qdrant.NewValueString("hash/v1"),
	}
	b := map[string]*qdrant.Value{
		"strategy":   qdrant.NewValueString("hash/v1"),
		"object_id":  qdrant.NewValueString("o1"),
		"created_at": qdrant.NewValueInt(1700000000),
	}

	for i := 0; i < 20; i++ {
		if HashPayload(a) != HashPayload(b) {
			t.Fatal("payload hash depends on key order")
		}
	}
	changed := map[string]*qdrant.Value{"object_id": qdrant.NewValueString("o2")}
	if HashPayload(a) == HashPayload(changed) {
		t.Error("a different payload must hash differently")
	}
}

func TestHashVectors_DetectsValueChanges(t *testing.T) {
	if HashVectors(nil) != "" {
		t.Error("a point with no vectors must hash to empty")
	}
	base := denseVectorsOutput(0.1, 0.2, 0.3)
	if HashVectors(base) != HashVectors(denseVectorsOutput(0.1, 0.2, 0.3)) {
		t.Error("identical vectors must hash identically")
	}
	if HashVectors(base) == HashVectors(denseVectorsOutput(0.1, 0.2)) {
		t.Error("a truncated vector must hash differently")
	}
	if HashVectors(base) == HashVectors(denseVectorsOutput(0.3, 0.2, 0.1)) {
		t.Error("reordered values must hash differently")
	}
}

// The copy is verified by reading it back because a dense vector may not be
// recomputable: if the upsert silently coerced it, the only chance to notice
// is before the original is deleted.
func TestCopyPointVerified_ProvesTheCopyLanded(t *testing.T) {
	store := newFakePointStore(10)
	store.add(9, "object_id", "o1", []float32{0.5, 0.25})

	if err := CopyPointVerified(context.Background(), store, "ns_objects_dense", 9, 20); err != nil {
		t.Fatalf("CopyPointVerified: %v", err)
	}
	copied, ok := store.points[20]
	if !ok {
		t.Fatal("the copy is missing")
	}
	if HashVectors(copied.GetVectors()) != HashVectors(store.points[9].GetVectors()) {
		t.Error("the copy does not match the source")
	}
	// The original is untouched — deletion is a separate, later step.
	if _, ok := store.points[9]; !ok {
		t.Error("the source point must survive the copy")
	}
}

func TestCopyPointVerified_FailsWhenTheCopyIsCorrupted(t *testing.T) {
	store := newFakePointStore(10)
	store.add(9, "object_id", "o1", []float32{0.5, 0.25})
	store.corruptOnUpsert = true

	err := CopyPointVerified(context.Background(), store, "ns_objects_dense", 9, 20)
	if err == nil {
		t.Fatal("a corrupted copy must not be reported as successful")
	}
	if !errorContains(err, "vector hash") {
		t.Errorf("error should name the failing check: %v", err)
	}
}

func TestCopyPointVerified_MissingSourceIsAnError(t *testing.T) {
	store := newFakePointStore(10)

	if err := CopyPointVerified(context.Background(), store, "ns_objects_dense", 9, 20); err == nil {
		t.Fatal("copying a point that does not exist must fail loudly")
	}
}

// An identity already on its authoritative id needs no work, and rewriting it
// would risk a correct point for nothing.
func TestCopyPointVerified_SameIDIsANoOp(t *testing.T) {
	store := newFakePointStore(10)
	store.upsertErr = errors.New("upsert must not be called")

	if err := CopyPointVerified(context.Background(), store, "ns_objects_dense", 9, 9); err != nil {
		t.Fatalf("same-id copy: %v", err)
	}
}

func TestDeletePointAndPointAbsent(t *testing.T) {
	store := newFakePointStore(10)
	store.add(9, "object_id", "o1", []float32{1, 0})

	absent, err := PointAbsent(context.Background(), store, "ns_objects_dense", 9)
	if err != nil || absent {
		t.Fatalf("point should be present: absent=%v err=%v", absent, err)
	}
	if err := DeletePoint(context.Background(), store, "ns_objects_dense", 9); err != nil {
		t.Fatalf("DeletePoint: %v", err)
	}
	absent, err = PointAbsent(context.Background(), store, "ns_objects_dense", 9)
	if err != nil || !absent {
		t.Fatalf("point should be gone: absent=%v err=%v", absent, err)
	}
}

func TestIsMissingCollection(t *testing.T) {
	if IsMissingCollection(nil) {
		t.Error("nil is not a missing collection")
	}
	for _, message := range []string{
		"Collection `ns_objects_dense` doesn't exist!",
		"collection does not exist",
		"NOT FOUND: collection",
	} {
		if !IsMissingCollection(errors.New(message)) {
			t.Errorf("%q should read as a missing collection", message)
		}
	}
	if IsMissingCollection(errors.New("connection refused")) {
		t.Error("a transport failure must not be mistaken for an absent collection")
	}
}

func errorContains(err error, want string) bool {
	return err != nil && want != "" && contains(err.Error(), want)
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// namedDense extracts the dense values from upsert input so the fake can store
// them in the same shape a real read returns.
//
// It reads through the oneof rather than the flat Data accessor for the same
// reason the production converter does: the legacy field is empty for
// everything Qdrant produces today.
func namedDense(vectors *qdrant.Vectors) []float32 {
	if named := vectors.GetVectors(); named != nil {
		for _, vector := range named.GetVectors() {
			return vector.GetDense().GetData()
		}
	}
	return vectors.GetVector().GetDense().GetData()
}
