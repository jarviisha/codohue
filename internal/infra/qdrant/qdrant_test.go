package qdrant

import (
	"context"
	"errors"
	"testing"

	qdrantpb "github.com/qdrant/go-client/qdrant"
)

func TestResolveDenseDistance(t *testing.T) {
	tests := []struct {
		input string
		want  qdrantpb.Distance
	}{
		{input: "dot", want: qdrantpb.Distance_Dot},
		{input: "DOT", want: qdrantpb.Distance_Dot},
		{input: "cosine", want: qdrantpb.Distance_Cosine},
		{input: "anything-else", want: qdrantpb.Distance_Cosine},
	}

	for _, tt := range tests {
		if got := resolveDenseDistance(tt.input); got != tt.want {
			t.Fatalf("resolveDenseDistance(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestCreateSparseCollection_UsesExpectedConfig(t *testing.T) {
	orig := createCollectionFn
	t.Cleanup(func() { createCollectionFn = orig })
	createCollectionFn = func(_ context.Context, _ *qdrantpb.Client, req *qdrantpb.CreateCollection) error {
		if req.CollectionName != "ns_objects" {
			t.Fatalf("collection name: got %q", req.CollectionName)
		}
		if req.SparseVectorsConfig == nil {
			t.Fatal("expected sparse vector config")
		}
		return nil
	}

	if err := createSparseCollection(context.Background(), nil, "ns_objects"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateDenseCollection_UsesExpectedConfig(t *testing.T) {
	orig := createCollectionFn
	t.Cleanup(func() { createCollectionFn = orig })
	createCollectionFn = func(_ context.Context, _ *qdrantpb.Client, req *qdrantpb.CreateCollection) error {
		if req.CollectionName != "ns_objects_dense" {
			t.Fatalf("collection name: got %q", req.CollectionName)
		}
		if req.VectorsConfig == nil {
			t.Fatal("expected dense vectors config")
		}
		return nil
	}

	if err := createDenseCollection(context.Background(), nil, "ns_objects_dense", 64, qdrantpb.Distance_Dot); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureCollections_CheckExistsError(t *testing.T) {
	origExists := collectionExistsFn
	t.Cleanup(func() { collectionExistsFn = origExists })
	collectionExistsFn = func(_ context.Context, _ *qdrantpb.Client, _ string) (bool, error) {
		return false, errors.New("exists failed")
	}

	if err := EnsureCollections(context.Background(), nil, "ns"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestEnsureCollections_CreatesMissingCollections(t *testing.T) {
	origExists := collectionExistsFn
	origCreate := createCollectionFn
	t.Cleanup(func() {
		collectionExistsFn = origExists
		createCollectionFn = origCreate
	})
	created := []string{}
	collectionExistsFn = func(_ context.Context, _ *qdrantpb.Client, name string) (bool, error) {
		return false, nil
	}
	createCollectionFn = func(_ context.Context, _ *qdrantpb.Client, req *qdrantpb.CreateCollection) error {
		created = append(created, req.CollectionName)
		return nil
	}

	if err := EnsureCollections(context.Background(), nil, "ns"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("expected 2 collections, got %v", created)
	}
}

func TestEnsureDenseCollections_CreateError(t *testing.T) {
	origExists := collectionExistsFn
	origCreate := createCollectionFn
	t.Cleanup(func() {
		collectionExistsFn = origExists
		createCollectionFn = origCreate
	})
	collectionExistsFn = func(_ context.Context, _ *qdrantpb.Client, _ string) (bool, error) {
		return false, nil
	}
	createCollectionFn = func(_ context.Context, _ *qdrantpb.Client, _ *qdrantpb.CreateCollection) error {
		return errors.New("create failed")
	}

	if err := EnsureDenseCollections(context.Background(), nil, "ns", 64, "dot"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestNewClient_Error(t *testing.T) {
	orig := newQdrantClientFn
	t.Cleanup(func() { newQdrantClientFn = orig })
	newQdrantClientFn = func(_ *qdrantpb.Config) (*qdrantpb.Client, error) {
		return nil, errors.New("dial failed")
	}

	if _, err := NewClient("localhost", 6334); err == nil {
		t.Fatal("expected error, got nil")
	}
}
