package embedder

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/core/namespace"
)

type fakeSweepRepo struct {
	pending     []StrandedItem
	pendingErr  error
	inflight    []StrandedItem
	inflightErr error

	listCalls  int
	resetCalls int
}

func (f *fakeSweepRepo) ListStrandedPending(_ context.Context, _ string, _ time.Time, _ int) ([]StrandedItem, error) {
	f.listCalls++
	return f.pending, f.pendingErr
}

func (f *fakeSweepRepo) ResetStrandedInFlight(_ context.Context, _ string, _ time.Time, _ int) ([]StrandedItem, error) {
	f.resetCalls++
	return f.inflight, f.inflightErr
}

type fakeSweepStream struct {
	groups  map[string][]redis.XInfoGroup
	infoErr error
	added   []*redis.XAddArgs
	addErr  error
}

func (f *fakeSweepStream) XInfoGroups(_ context.Context, stream string) *redis.XInfoGroupsCmd {
	cmd := redis.NewXInfoGroupsCmd(context.Background(), stream)
	if f.infoErr != nil {
		cmd.SetErr(f.infoErr)
		return cmd
	}
	cmd.SetVal(f.groups[stream])
	return cmd
}

func (f *fakeSweepStream) XAdd(_ context.Context, a *redis.XAddArgs) *redis.StringCmd {
	cmd := redis.NewStringCmd(context.Background(), "XADD")
	if f.addErr != nil {
		cmd.SetErr(f.addErr)
		return cmd
	}
	f.added = append(f.added, a)
	cmd.SetVal("1-0")
	return cmd
}

func sweeperWith(repo sweepRepo, rdb sweepStream, ns string) *RecoverySweeper {
	lister := &fakeNsLister{configs: []*namespace.Config{{
		Namespace:              ns,
		CatalogStrategyID:      "hashing",
		CatalogStrategyVersion: "v1",
	}}}
	return newRecoverySweeperWithDeps(repo, rdb, lister, RecoverySweeperConfig{})
}

func drainedGroups(ns string) map[string][]redis.XInfoGroup {
	return map[string][]redis.XInfoGroup{
		streamName(ns): {{Name: defaultConsumerGroup, Lag: 0, Pending: 0}},
	}
}

func TestSweeperRepublishesStrandedPending(t *testing.T) {
	repo := &fakeSweepRepo{pending: []StrandedItem{{ID: 7, ObjectID: "obj-7"}}}
	rdb := &fakeSweepStream{groups: drainedGroups("ns")}

	sweeperWith(repo, rdb, "ns").tick(context.Background())

	if len(rdb.added) != 1 {
		t.Fatalf("expected 1 republish, got %d", len(rdb.added))
	}
	args := rdb.added[0]
	if args.Stream != streamName("ns") {
		t.Errorf("stream: got %q", args.Stream)
	}
	if got := args.Values.(map[string]any)["catalog_item_id"]; got != int64(7) {
		t.Errorf("catalog_item_id: got %v", got)
	}
	if got := args.Values.(map[string]any)["strategy_id"]; got != "hashing" {
		t.Errorf("strategy_id: got %v", got)
	}
}

func TestSweeperSkipsWhenStreamNotDrained(t *testing.T) {
	for name, group := range map[string]redis.XInfoGroup{
		"undelivered entries": {Name: defaultConsumerGroup, Lag: 3, Pending: 0},
		"pel entries":         {Name: defaultConsumerGroup, Lag: 0, Pending: 2},
	} {
		repo := &fakeSweepRepo{pending: []StrandedItem{{ID: 7, ObjectID: "obj-7"}}}
		rdb := &fakeSweepStream{groups: map[string][]redis.XInfoGroup{
			streamName("ns"): {group},
		}}

		sweeperWith(repo, rdb, "ns").tick(context.Background())

		if repo.listCalls != 0 || repo.resetCalls != 0 || len(rdb.added) != 0 {
			t.Errorf("%s: expected no sweep activity, got list=%d reset=%d added=%d",
				name, repo.listCalls, repo.resetCalls, len(rdb.added))
		}
	}
}

func TestSweeperTreatsMissingStreamAsDrained(t *testing.T) {
	repo := &fakeSweepRepo{pending: []StrandedItem{{ID: 1, ObjectID: "o1"}}}
	rdb := &fakeSweepStream{infoErr: errors.New("ERR no such key")}

	sweeperWith(repo, rdb, "ns").tick(context.Background())

	if len(rdb.added) != 1 {
		t.Fatalf("expected republish for missing stream, got %d", len(rdb.added))
	}
}

func TestSweeperTreatsMissingGroupAsDrained(t *testing.T) {
	repo := &fakeSweepRepo{pending: []StrandedItem{{ID: 1, ObjectID: "o1"}}}
	rdb := &fakeSweepStream{groups: map[string][]redis.XInfoGroup{
		streamName("ns"): {{Name: "some-other-group", Lag: 9, Pending: 9}},
	}}

	sweeperWith(repo, rdb, "ns").tick(context.Background())

	if len(rdb.added) != 1 {
		t.Fatalf("expected republish when consumer group is missing, got %d", len(rdb.added))
	}
}

func TestSweeperRepublishesResetInFlight(t *testing.T) {
	repo := &fakeSweepRepo{
		pending:  []StrandedItem{{ID: 1, ObjectID: "o1"}},
		inflight: []StrandedItem{{ID: 2, ObjectID: "o2"}},
	}
	rdb := &fakeSweepStream{groups: drainedGroups("ns")}

	sweeperWith(repo, rdb, "ns").tick(context.Background())

	if len(rdb.added) != 2 {
		t.Fatalf("expected pending + reset in_flight republished, got %d", len(rdb.added))
	}
}

func TestSweeperResetErrorStillRepublishesPending(t *testing.T) {
	repo := &fakeSweepRepo{
		pending:     []StrandedItem{{ID: 1, ObjectID: "o1"}},
		inflightErr: errors.New("db error"),
	}
	rdb := &fakeSweepStream{groups: drainedGroups("ns")}

	sweeperWith(repo, rdb, "ns").tick(context.Background())

	if len(rdb.added) != 1 {
		t.Fatalf("expected pending republished despite reset error, got %d", len(rdb.added))
	}
}

func TestSweeperToleratesPublishError(t *testing.T) {
	repo := &fakeSweepRepo{pending: []StrandedItem{{ID: 1, ObjectID: "o1"}}}
	rdb := &fakeSweepStream{groups: drainedGroups("ns"), addErr: errors.New("redis down")}

	// Must not panic; the rows stay pending for the next tick.
	sweeperWith(repo, rdb, "ns").tick(context.Background())
}

func TestSweeperToleratesListerError(t *testing.T) {
	sweeper := newRecoverySweeperWithDeps(
		&fakeSweepRepo{}, &fakeSweepStream{},
		&fakeNsLister{err: errors.New("db down")},
		RecoverySweeperConfig{},
	)
	sweeper.tick(context.Background())
}

func TestSweeperConfigDefaults(t *testing.T) {
	s := newRecoverySweeperWithDeps(&fakeSweepRepo{}, &fakeSweepStream{}, &fakeNsLister{}, RecoverySweeperConfig{})
	if s.cfg.Interval != 2*time.Minute {
		t.Errorf("Interval: got %v", s.cfg.Interval)
	}
	if s.cfg.PendingStaleAfter != 5*time.Minute {
		t.Errorf("PendingStaleAfter: got %v", s.cfg.PendingStaleAfter)
	}
	if s.cfg.InFlightStaleAfter != 15*time.Minute {
		t.Errorf("InFlightStaleAfter: got %v", s.cfg.InFlightStaleAfter)
	}
	if s.cfg.BatchSize != 500 {
		t.Errorf("BatchSize: got %d", s.cfg.BatchSize)
	}
}
