package main

import (
	"context"
	"testing"

	"github.com/jarviisha/codohue/internal/admin"
	"github.com/jarviisha/codohue/internal/nsconfig"
)

type fakeNsUpsertSvc struct {
	resp   *nsconfig.UpsertResponse
	err    error
	gotReq *nsconfig.UpsertRequest
}

func (f *fakeNsUpsertSvc) Upsert(_ context.Context, _ string, req *nsconfig.UpsertRequest) (*nsconfig.UpsertResponse, error) {
	f.gotReq = req
	return f.resp, f.err
}

// TestNsConfigAdapter_Upsert_MapsDenseSource pins the admin → nsconfig
// dense_source mapping. There is no conflict error to surface — the producer
// is a single field.
func TestNsConfigAdapter_Upsert_MapsDenseSource(t *testing.T) {
	fake := &fakeNsUpsertSvc{resp: &nsconfig.UpsertResponse{Namespace: "ns"}}
	a := &nsConfigAdapter{svc: fake}

	src := "item2vec"
	_, err := a.Upsert(context.Background(), "ns", &admin.NamespaceUpsertRequest{DenseSource: &src})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.gotReq == nil || fake.gotReq.DenseSource != "item2vec" {
		t.Errorf("dense_source not mapped: %+v", fake.gotReq)
	}
}
