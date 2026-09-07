package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	goredis "github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/admin"
)

type fakeBacklogRedis struct {
	streams []string
}

func (f *fakeBacklogRedis) XLen(ctx context.Context, stream string) *goredis.IntCmd {
	f.streams = append(f.streams, stream)
	cmd := goredis.NewIntCmd(ctx)
	cmd.SetVal(0)
	return cmd
}

func (f *fakeBacklogRedis) XInfoGroups(ctx context.Context, stream string) *goredis.XInfoGroupsCmd {
	f.streams = append(f.streams, stream)
	cmd := goredis.NewXInfoGroupsCmd(ctx, stream)
	cmd.SetVal(nil)
	return cmd
}

type fakeStateCounter struct {
	counts admin.CatalogItemStateCounts
	err    error
	calls  []string
}

func (f *fakeStateCounter) CountCatalogItemStates(_ context.Context, namespace string) (admin.CatalogItemStateCounts, error) {
	f.calls = append(f.calls, namespace)
	return f.counts, f.err
}

func TestCatalogBacklogAdapter_ReadMapsCountsWithoutRedis(t *testing.T) {
	counter := &fakeStateCounter{
		counts: admin.CatalogItemStateCounts{
			Pending:    3,
			InFlight:   1,
			Embedded:   42,
			Failed:     2,
			DeadLetter: 7,
		},
	}
	adapter := newCatalogBacklogAdapter(counter, nil)

	got, err := adapter.Read(context.Background(), "ns_a", 1)
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}

	want := admin.CatalogBacklog{Pending: 3, InFlight: 1, Embedded: 42, Failed: 2, DeadLetter: 7, StreamLen: 0}
	if got != want {
		t.Fatalf("backlog mismatch:\n got=%+v\nwant=%+v", got, want)
	}
	if len(counter.calls) != 1 || counter.calls[0] != "ns_a" {
		t.Fatalf("expected one call with ns_a, got %v", counter.calls)
	}
}

func TestCatalogBacklogAdapter_ReadPropagatesCounterError(t *testing.T) {
	wantErr := errors.New("db is down")
	adapter := newCatalogBacklogAdapter(&fakeStateCounter{err: wantErr}, nil)

	_, err := adapter.Read(context.Background(), "ns_a", 1)
	if err == nil || !errors.Is(err, wantErr) {
		t.Fatalf("expected wrapped %v, got %v", wantErr, err)
	}
}

func TestCatalogBacklogAdapter_ReadUsesCurrentGenerationStream(t *testing.T) {
	redis := &fakeBacklogRedis{}
	adapter := newCatalogBacklogAdapter(&fakeStateCounter{}, redis)

	if _, err := adapter.Read(context.Background(), "ns_a", 3); err != nil {
		t.Fatalf("Read returned error: %v", err)
	}
	want := []string{"catalog:embed:ns_a:g3", "catalog:embed:ns_a:g3"}
	if fmt.Sprint(redis.streams) != fmt.Sprint(want) {
		t.Fatalf("streams = %v, want %v", redis.streams, want)
	}
}
