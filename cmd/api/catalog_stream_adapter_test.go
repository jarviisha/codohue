package main

import (
	"context"
	"errors"
	"testing"

	"github.com/jarviisha/codohue/internal/catalog"
	"github.com/jarviisha/codohue/internal/core/namespace"
	"github.com/jarviisha/codohue/internal/ingest"
	"github.com/jarviisha/codohue/pkg/codohuetypes"
)

type adapterFakeNsCfg struct {
	cfg *namespace.Config
	err error
}

func (f *adapterFakeNsCfg) Get(_ context.Context, _ string) (*namespace.Config, error) {
	return f.cfg, f.err
}

// The adapter's real job is error classification: validation failures must
// come back wrapped in ingest.ErrCatalogItemRejected (permanent → acked off
// the stream), infra failures must not (transient → redelivered).
func TestCatalogStreamAdapter_ClassifiesValidationAsRejected(t *testing.T) {
	a := &catalogStreamAdapter{svc: catalog.NewService(nil, &adapterFakeNsCfg{}, nil)}

	// Empty content fails validation before any dependency is touched.
	err := a.IngestStreamItem(context.Background(), &codohuetypes.CatalogStreamItem{
		Namespace: "ns", ObjectID: "o1", Content: "   ",
	})
	if !errors.Is(err, ingest.ErrCatalogItemRejected) {
		t.Fatalf("empty content must classify as rejected, got %v", err)
	}

	// Unknown namespace (nil config) is also permanent.
	err = a.IngestStreamItem(context.Background(), &codohuetypes.CatalogStreamItem{
		Namespace: "ghost", ObjectID: "o1", Content: "hello",
	})
	if !errors.Is(err, ingest.ErrCatalogItemRejected) {
		t.Fatalf("unknown namespace must classify as rejected, got %v", err)
	}
}

func TestCatalogStreamAdapter_InfraFailureStaysTransient(t *testing.T) {
	a := &catalogStreamAdapter{svc: catalog.NewService(nil, &adapterFakeNsCfg{err: errors.New("db down")}, nil)}

	err := a.IngestStreamItem(context.Background(), &codohuetypes.CatalogStreamItem{
		Namespace: "ns", ObjectID: "o1", Content: "hello",
	})
	if err == nil || errors.Is(err, ingest.ErrCatalogItemRejected) {
		t.Fatalf("config lookup failure must stay transient, got %v", err)
	}
}
