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

// TestNsConfigAdapter_Upsert_MapsDenseSource pins the dense_source → legacy
// dense_strategy mapping used during the dual-write window. There is no longer
// any conflict error to surface — the producer is a single field.
func TestNsConfigAdapter_Upsert_MapsDenseSource(t *testing.T) {
	fake := &fakeNsUpsertSvc{resp: &nsconfig.UpsertResponse{Namespace: "ns"}}
	a := &nsConfigAdapter{svc: fake}

	src := "item2vec"
	_, err := a.Upsert(context.Background(), "ns", &admin.NamespaceUpsertRequest{DenseSource: &src})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.gotReq == nil || fake.gotReq.DenseStrategy != "item2vec" {
		t.Errorf("dense_source not mapped onto dense_strategy: %+v", fake.gotReq)
	}
}
