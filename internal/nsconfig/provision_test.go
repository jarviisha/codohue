package nsconfig

import (
	"context"
	"strings"
	"testing"

	"github.com/jarviisha/codohue/internal/core/embedstrategy"
)

func TestProvisionRetryPreservesChangedEmbeddingDimension(t *testing.T) {
	for _, explicitDim := range []bool{false, true} {
		name := "implicit"
		if explicitDim {
			name = "explicit"
		}
		t.Run(name, func(t *testing.T) {
			db := openTestDB(t)
			ns := "nsconfig_provision_dim_" + name
			cleanupNS(t, db, ns)
			ctx := context.Background()
			repo := NewRepository(db)
			svc := NewService(repo)
			svc.registry = embedstrategy.NewRegistry()
			svc.registry.Register("internal-hashing-ngrams", "v1", func(params embedstrategy.Params) (embedstrategy.Strategy, error) {
				return &stubStrategyT{id: "internal-hashing-ngrams", version: "v1", dim: params["dim"].(int)}, nil
			})
			checker := &fakeDenseChecker{}
			svc.SetDenseCollectionChecker(checker)
			req := &UpsertRequest{
				ProvisionAPIKey: strings.Repeat("ab", 32),
				DenseSource:     ptr("catalog"), CatalogStrategyID: ptr("internal-hashing-ngrams"),
				CatalogStrategyVersion: ptr("v1"), CatalogStrategyParams: map[string]any{"dim": 64},
			}
			if explicitDim {
				req.EmbeddingDim = ptr(64)
			}
			if _, err := svc.Upsert(ctx, ns, req); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.Upsert(ctx, ns, &UpsertRequest{
				EmbeddingDim: ptr(128), DenseSource: ptr("catalog"),
				CatalogStrategyID: ptr("internal-hashing-ngrams"), CatalogStrategyVersion: ptr("v1"),
				CatalogStrategyParams: map[string]any{"dim": 128},
			}); err != nil {
				t.Fatal(err)
			}
			checker.exists = true
			if _, err := svc.Upsert(ctx, ns, req); err != nil {
				t.Fatalf("unchanged provisioning retry after operator edit: %v", err)
			}
			got, err := repo.Get(ctx, ns)
			if err != nil {
				t.Fatal(err)
			}
			if got.EmbeddingDim != 128 || got.CatalogStrategyParams["dim"] != float64(128) {
				t.Fatal("provisioning retry overwrote the current dimension or strategy")
			}
		})
	}
}
