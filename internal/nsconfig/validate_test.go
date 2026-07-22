package nsconfig

import (
	"context"
	"errors"
	"testing"

	"github.com/jarviisha/codohue/internal/core/namespace"
)

func fptr(v float64) *float64 { return &v }
func iptr(v int) *int         { return &v }
func sptr(v string) *string   { return &v }

func TestValidateUpsert_RangeChecks(t *testing.T) {
	cases := []struct {
		name string
		req  *UpsertRequest
		ok   bool
	}{
		{"nil request", nil, true},
		{"empty request", &UpsertRequest{}, true},
		{"valid full", &UpsertRequest{
			ActionWeights: map[string]float64{"click": 1, "like": 3},
			Lambda:        fptr(0.05), Gamma: fptr(0), Alpha: fptr(0.7),
			MaxResults: iptr(50), SeenItemsDays: iptr(30),
			DenseSource: sptr("item2vec"), EmbeddingDim: iptr(64), DenseDistance: sptr("cosine"),
			TrendingWindow: iptr(24), TrendingTTL: iptr(600), LambdaTrending: fptr(0.1),
		}, true},
		{"negative action weight", &UpsertRequest{ActionWeights: map[string]float64{"click": -1}}, false},
		{"zero lambda", &UpsertRequest{Lambda: fptr(0)}, false},
		{"negative gamma", &UpsertRequest{Gamma: fptr(-0.1)}, false},
		{"zero max_results", &UpsertRequest{MaxResults: iptr(0)}, false},
		{"zero seen_items_days", &UpsertRequest{SeenItemsDays: iptr(0)}, false},
		{"alpha below zero", &UpsertRequest{Alpha: fptr(-0.1)}, false},
		{"alpha above one", &UpsertRequest{Alpha: fptr(1.5)}, false},
		{"alpha bounds inclusive", &UpsertRequest{Alpha: fptr(0)}, true},
		{"unknown dense_source", &UpsertRequest{DenseSource: sptr("magic")}, false},
		{"zero embedding_dim", &UpsertRequest{EmbeddingDim: iptr(0)}, false},
		{"unknown dense_distance", &UpsertRequest{DenseDistance: sptr("manhattan")}, false},
		{"zero trending_window", &UpsertRequest{TrendingWindow: iptr(0)}, false},
		{"zero trending_ttl", &UpsertRequest{TrendingTTL: iptr(0)}, false},
		{"zero lambda_trending", &UpsertRequest{LambdaTrending: fptr(0)}, false},
	}
	for _, tc := range cases {
		err := validateUpsert(tc.req)
		if tc.ok && err != nil {
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		}
		if !tc.ok {
			if err == nil {
				t.Errorf("%s: expected validation error", tc.name)
			} else if !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("%s: expected ErrInvalidConfig, got %v", tc.name, err)
			}
		}
	}
}

func TestValidateUpsert_CatalogViaUpsertRejected(t *testing.T) {
	// Flipping dense_source=catalog through the generic PATCH bypassed the
	// strategy-vs-dim validation and wedged the namespace: the embedder
	// dead-lettered every item while BYOE writes 409'd.
	err := validateUpsert(&UpsertRequest{DenseSource: sptr("catalog")})
	if !errors.Is(err, ErrCatalogViaUpsert) {
		t.Fatalf("expected ErrCatalogViaUpsert, got %v", err)
	}
}

type fakeDenseChecker struct {
	exists bool
	err    error
	calls  int
}

func (f *fakeDenseChecker) DenseCollectionsExist(_ context.Context, _ string) (bool, error) {
	f.calls++
	return f.exists, f.err
}

func TestUpsert_EmbeddingDimChangeLockedWhileCollectionsExist(t *testing.T) {
	repo := &fakeRepo{getCfg: &namespace.Config{Namespace: "ns", EmbeddingDim: 64}}
	svc := NewService(nil)
	svc.repo = repo
	svc.SetDenseCollectionChecker(&fakeDenseChecker{exists: true})

	_, err := svc.Upsert(context.Background(), "ns", &UpsertRequest{EmbeddingDim: iptr(128)})
	if !errors.Is(err, ErrEmbeddingDimLocked) {
		t.Fatalf("expected ErrEmbeddingDimLocked, got %v — a changed dim fails every dense upsert forever", err)
	}
}

func TestUpsert_EmbeddingDimChangeAllowedWithoutCollections(t *testing.T) {
	repo := &fakeRepo{
		getCfg:    &namespace.Config{Namespace: "ns", EmbeddingDim: 64},
		upsertCfg: &namespace.Config{Namespace: "ns", APIKeyHash: "h"},
	}
	svc := NewService(nil)
	svc.repo = repo
	svc.SetDenseCollectionChecker(&fakeDenseChecker{exists: false})

	if _, err := svc.Upsert(context.Background(), "ns", &UpsertRequest{EmbeddingDim: iptr(128)}); err != nil {
		t.Fatalf("dim change with no collections must pass: %v", err)
	}
}

func TestUpsert_SameDimSkipsCollectionCheck(t *testing.T) {
	checker := &fakeDenseChecker{exists: true}
	repo := &fakeRepo{
		getCfg:    &namespace.Config{Namespace: "ns", EmbeddingDim: 64},
		upsertCfg: &namespace.Config{Namespace: "ns", APIKeyHash: "h"},
	}
	svc := NewService(nil)
	svc.repo = repo
	svc.SetDenseCollectionChecker(checker)

	if _, err := svc.Upsert(context.Background(), "ns", &UpsertRequest{EmbeddingDim: iptr(64)}); err != nil {
		t.Fatalf("a no-op dim must pass: %v", err)
	}
	if checker.calls != 0 {
		t.Fatal("no-op dim must not pay the Qdrant round-trip")
	}
}
