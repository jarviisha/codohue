package recommend

import (
	"errors"
	"testing"

	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

func TestMapDeleteError(t *testing.T) {
	t.Parallel()
	if err := mapDeleteError("ns_objects", nil); err != nil {
		t.Fatalf("nil error must stay nil, got %v", err)
	}
	// A missing collection is a successful no-op: the object never had a
	// vector there (e.g. the cron job hasn't run yet).
	if err := mapDeleteError("ns_objects", grpcstatus.Error(codes.NotFound, "missing")); err != nil {
		t.Fatalf("NotFound must be a no-op, got %v", err)
	}
	err := mapDeleteError("ns_objects", errors.New("delete failed"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got, want := err.Error(), `delete from "ns_objects": qdrant delete: delete failed`; got != want {
		t.Fatalf("error text: got %q want %q", got, want)
	}
}

func TestSparseVectorOf(t *testing.T) {
	t.Parallel()
	points := []*qdrant.RetrievedPoint{{
		Vectors: &qdrant.VectorsOutput{
			VectorsOptions: &qdrant.VectorsOutput_Vectors{
				Vectors: &qdrant.NamedVectorsOutput{
					Vectors: map[string]*qdrant.VectorOutput{
						sparseVectorName: {Vector: &qdrant.VectorOutput_Sparse{Sparse: &qdrant.SparseVector{Indices: []uint32{1}, Values: []float32{2}}}},
					},
				},
			},
		},
	}}

	vec := sparseVectorOf(points)
	if vec == nil || len(vec.Indices) != 1 || vec.Indices[0] != 1 || vec.Values[0] != 2 {
		t.Fatalf("unexpected vector: %+v", vec)
	}
	if sparseVectorOf(nil) != nil {
		t.Fatal("no points must yield nil")
	}
	if sparseVectorOf([]*qdrant.RetrievedPoint{{}}) != nil {
		t.Fatal("a point without the named vector must yield nil")
	}
}

func TestDenseVectorOf(t *testing.T) {
	t.Parallel()
	points := []*qdrant.RetrievedPoint{{
		Vectors: &qdrant.VectorsOutput{
			VectorsOptions: &qdrant.VectorsOutput_Vectors{
				Vectors: &qdrant.NamedVectorsOutput{
					Vectors: map[string]*qdrant.VectorOutput{
						denseVectorName: {Vector: &qdrant.VectorOutput_Dense{Dense: &qdrant.DenseVector{Data: []float32{0.1, 0.2}}}},
					},
				},
			},
		},
	}}

	vec := denseVectorOf(points)
	if len(vec) != 2 || vec[0] != 0.1 {
		t.Fatalf("unexpected vector: %+v", vec)
	}
	if denseVectorOf(nil) != nil {
		t.Fatal("no points must yield nil")
	}
	if denseVectorOf([]*qdrant.RetrievedPoint{{}}) != nil {
		t.Fatal("a point without the named vector must yield nil")
	}
}
