package main

import (
	"context"
	"errors"
	"testing"

	"github.com/jarviisha/codohue/internal/admin"
	"github.com/jarviisha/codohue/internal/core/embedstrategy"
	"github.com/jarviisha/codohue/internal/core/namespace"
	"github.com/jarviisha/codohue/internal/nsconfig"
)

type fakeNsCatalogSvc struct {
	getResp    *namespace.Config
	getErr     error
	updateResp *namespace.Config
	updateErr  error
	gotReq     *nsconfig.UpdateCatalogRequest
}

func (f *fakeNsCatalogSvc) Get(_ context.Context, _ string) (*namespace.Config, error) {
	return f.getResp, f.getErr
}

func (f *fakeNsCatalogSvc) UpdateCatalogConfig(_ context.Context, _ string, req *nsconfig.UpdateCatalogRequest) (*namespace.Config, error) {
	f.gotReq = req
	return f.updateResp, f.updateErr
}

func newTestCatalogAdapter(svc nsCatalogConfigSvc) *catalogConfigAdapter {
	return newCatalogConfigAdapter(svc, embedstrategy.NewRegistry())
}

// TestCatalogConfigAdapter_UpdateCatalog_MapsDenseStrategyConflict pins the
// bridge from the domain-level *nsconfig.DenseStrategyConflictError to
// admin.CatalogStrategyConflict so the catalog handler can render the
// dense_strategy_conflict body.
func TestCatalogConfigAdapter_UpdateCatalog_MapsDenseStrategyConflict(t *testing.T) {
	a := newTestCatalogAdapter(&fakeNsCatalogSvc{
		updateErr: &nsconfig.DenseStrategyConflictError{
			DenseStrategy:  "svd",
			CatalogEnabled: true,
		},
	})

	_, err := a.UpdateCatalog(context.Background(), "ns", &admin.NamespaceCatalogUpdateRequest{Enabled: true})

	var conflictErr *admin.CatalogStrategyConflict
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected *admin.CatalogStrategyConflict, got %v (%T)", err, err)
	}
	if conflictErr.DenseStrategy != "svd" {
		t.Errorf("DenseStrategy = %q, want svd", conflictErr.DenseStrategy)
	}
	if !conflictErr.CatalogEnabled {
		t.Errorf("CatalogEnabled = false, want true")
	}
}

// TestCatalogConfigAdapter_UpdateCatalog_MapsDimensionMismatch makes sure the
// existing dim-mismatch mapping survived the addition of the conflict branch.
func TestCatalogConfigAdapter_UpdateCatalog_MapsDimensionMismatch(t *testing.T) {
	a := newTestCatalogAdapter(&fakeNsCatalogSvc{
		updateErr: &nsconfig.DimensionMismatchError{StrategyDim: 64, NamespaceEmbeddingDim: 128},
	})

	_, err := a.UpdateCatalog(context.Background(), "ns", &admin.NamespaceCatalogUpdateRequest{Enabled: true})

	var dimErr *admin.CatalogDimensionMismatch
	if !errors.As(err, &dimErr) {
		t.Fatalf("expected *admin.CatalogDimensionMismatch, got %v (%T)", err, err)
	}
	if dimErr.StrategyDim != 64 || dimErr.NamespaceEmbeddingDim != 128 {
		t.Errorf("dims mis-mapped: %+v", dimErr)
	}
}

// TestCatalogConfigAdapter_UpdateCatalog_NotFoundReturnsNilNil collapses
// ErrNamespaceNotFound into a nil-result so the admin handler emits 404.
func TestCatalogConfigAdapter_UpdateCatalog_NotFoundReturnsNilNil(t *testing.T) {
	a := newTestCatalogAdapter(&fakeNsCatalogSvc{updateErr: nsconfig.ErrNamespaceNotFound})

	cfg, err := a.UpdateCatalog(context.Background(), "ns", &admin.NamespaceCatalogUpdateRequest{Enabled: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil config when ns missing, got %+v", cfg)
	}
}

// TestCatalogConfigAdapter_UpdateCatalog_PassesThroughOther confirms an
// unknown error bubbles up unchanged so the handler's default 400 branch
// still surfaces something actionable (e.g. embedstrategy.ErrUnknownStrategy).
func TestCatalogConfigAdapter_UpdateCatalog_PassesThroughOther(t *testing.T) {
	want := errors.New("registry: unknown strategy")
	a := newTestCatalogAdapter(&fakeNsCatalogSvc{updateErr: want})

	_, err := a.UpdateCatalog(context.Background(), "ns", &admin.NamespaceCatalogUpdateRequest{Enabled: true})
	if !errors.Is(err, want) {
		t.Fatalf("expected raw error to pass through, got %v", err)
	}
}
