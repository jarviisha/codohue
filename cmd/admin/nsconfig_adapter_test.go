package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jarviisha/codohue/internal/admin"
	"github.com/jarviisha/codohue/internal/nsconfig"
)

type fakeNsUpsertSvc struct {
	resp *nsconfig.UpsertResponse
	err  error
}

func (f *fakeNsUpsertSvc) Upsert(_ context.Context, _ string, _ *nsconfig.UpsertRequest) (*nsconfig.UpsertResponse, error) {
	return f.resp, f.err
}

// TestNsConfigAdapter_Upsert_MapsDenseStrategyConflict pins the bridge from
// the domain-level *nsconfig.DenseStrategyConflictError to the admin-level
// *admin.CatalogStrategyConflict. Without this mapping the admin handler
// would surface every conflict as a generic 500.
func TestNsConfigAdapter_Upsert_MapsDenseStrategyConflict(t *testing.T) {
	a := &nsConfigAdapter{svc: &fakeNsUpsertSvc{
		err: &nsconfig.DenseStrategyConflictError{
			DenseStrategy:  "item2vec",
			CatalogEnabled: true,
		},
	}}

	_, err := a.Upsert(context.Background(), "ns", &admin.NamespaceUpsertRequest{})

	var conflictErr *admin.CatalogStrategyConflict
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected *admin.CatalogStrategyConflict, got %v (%T)", err, err)
	}
	if conflictErr.DenseStrategy != "item2vec" {
		t.Errorf("DenseStrategy = %q, want item2vec", conflictErr.DenseStrategy)
	}
	if !conflictErr.CatalogEnabled {
		t.Errorf("CatalogEnabled = false, want true")
	}
}

// TestNsConfigAdapter_Upsert_PassesThroughOtherErrors guards against a regression
// where adding the conflict branch swallows unrelated errors.
func TestNsConfigAdapter_Upsert_PassesThroughOtherErrors(t *testing.T) {
	want := errors.New("db unreachable")
	a := &nsConfigAdapter{svc: &fakeNsUpsertSvc{err: want}}

	_, err := a.Upsert(context.Background(), "ns", &admin.NamespaceUpsertRequest{})
	if !errors.Is(err, want) {
		t.Fatalf("expected raw error to pass through, got %v", err)
	}
}

// TestNsConfigAdapter_Upsert_HappyPath confirms the pointer→value DTO
// translation still works after the error-mapping refactor.
func TestNsConfigAdapter_Upsert_HappyPath(t *testing.T) {
	key := "plaintext-key"
	updated := time.Now()
	a := &nsConfigAdapter{svc: &fakeNsUpsertSvc{
		resp: &nsconfig.UpsertResponse{
			Namespace: "ns",
			UpdatedAt: updated,
			APIKey:    key,
		},
	}}

	got, err := a.Upsert(context.Background(), "ns", &admin.NamespaceUpsertRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Namespace != "ns" {
		t.Errorf("Namespace = %q, want ns", got.Namespace)
	}
	if got.APIKey == nil || *got.APIKey != key {
		t.Errorf("APIKey not propagated: %+v", got.APIKey)
	}
}
