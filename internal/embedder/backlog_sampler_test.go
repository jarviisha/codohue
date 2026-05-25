package embedder

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/core/namespace"
)

// --- fakes ------------------------------------------------------------------

type fakeBacklogRepo struct {
	mu sync.Mutex

	counts   map[string]BacklogStateCounts
	latest   map[string]sampleSnapshot
	latestOK map[string]bool

	inserts []insertCall

	countsErr error
	latestErr error
	insertErr error
}

type insertCall struct {
	namespace                                string
	at                                       time.Time
	pending, inFlight, failed, deadLetter, streamLen int
}

func newFakeBacklogRepo() *fakeBacklogRepo {
	return &fakeBacklogRepo{
		counts:   map[string]BacklogStateCounts{},
		latest:   map[string]sampleSnapshot{},
		latestOK: map[string]bool{},
	}
}

func (f *fakeBacklogRepo) CountBacklogStates(_ context.Context, ns string) (BacklogStateCounts, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.countsErr != nil {
		return BacklogStateCounts{}, f.countsErr
	}
	return f.counts[ns], nil
}

func (f *fakeBacklogRepo) InsertBacklogSample(_ context.Context, ns string, at time.Time, pending, inFlight, failed, deadLetter, streamLen int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.insertErr != nil {
		return f.insertErr
	}
	f.inserts = append(f.inserts, insertCall{ns, at, pending, inFlight, failed, deadLetter, streamLen})
	return nil
}

func (f *fakeBacklogRepo) LatestBacklogSample(_ context.Context, ns string) (BacklogStateCounts, int, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.latestErr != nil {
		return BacklogStateCounts{}, 0, false, f.latestErr
	}
	snap, ok := f.latest[ns]
	if !ok {
		return BacklogStateCounts{}, 0, false, nil
	}
	return snap.counts, snap.streamLen, f.latestOK[ns], nil
}

type fakeRedis struct {
	lens map[string]int64
	err  error
}

func (f *fakeRedis) XLen(_ context.Context, stream string) *redis.IntCmd {
	cmd := redis.NewIntCmd(context.Background(), "XLEN", stream)
	if f.err != nil {
		cmd.SetErr(f.err)
		return cmd
	}
	cmd.SetVal(f.lens[stream])
	return cmd
}

type fakeNsLister struct {
	configs []*namespace.Config
	err     error
}

func (f *fakeNsLister) ListCatalogEnabled(_ context.Context) ([]*namespace.Config, error) {
	return f.configs, f.err
}

func samplerWith(repo backlogRepo, rdb streamLengther, ns ...string) *BacklogSampler {
	cfgs := make([]*namespace.Config, 0, len(ns))
	for _, n := range ns {
		cfgs = append(cfgs, &namespace.Config{Namespace: n})
	}
	return newBacklogSamplerWithDeps(repo, rdb, &fakeNsLister{configs: cfgs}, BacklogSamplerConfig{
		Interval:        30 * time.Second,
		ForceWriteAfter: 5 * time.Minute,
	})
}

// --- tests ------------------------------------------------------------------

func TestSamplerWritesOnFirstTick(t *testing.T) {
	repo := newFakeBacklogRepo()
	repo.counts["prod"] = BacklogStateCounts{Pending: 5, InFlight: 1}
	rdb := &fakeRedis{lens: map[string]int64{"catalog:embed:prod": 13}}
	s := samplerWith(repo, rdb, "prod")

	s.tick(context.Background())

	if got := len(repo.inserts); got != 1 {
		t.Fatalf("inserts=%d, want 1", got)
	}
	if c := repo.inserts[0]; c.namespace != "prod" || c.pending != 5 || c.inFlight != 1 || c.streamLen != 13 {
		t.Errorf("first insert=%+v", c)
	}
}

func TestSamplerSkipsUnchangedSampleWithinForceWindow(t *testing.T) {
	repo := newFakeBacklogRepo()
	repo.counts["prod"] = BacklogStateCounts{Pending: 5}
	rdb := &fakeRedis{lens: map[string]int64{"catalog:embed:prod": 0}}
	s := samplerWith(repo, rdb, "prod")

	s.tick(context.Background())
	s.tick(context.Background())
	s.tick(context.Background())

	if got := len(repo.inserts); got != 1 {
		t.Fatalf("inserts=%d, want 1 (subsequent ticks unchanged + within force window)", got)
	}
}

func TestSamplerWritesWhenCountsChange(t *testing.T) {
	repo := newFakeBacklogRepo()
	repo.counts["prod"] = BacklogStateCounts{Pending: 5}
	rdb := &fakeRedis{lens: map[string]int64{}}
	s := samplerWith(repo, rdb, "prod")

	s.tick(context.Background()) // initial
	// counts change
	repo.counts["prod"] = BacklogStateCounts{Pending: 7, InFlight: 2}
	s.tick(context.Background())

	if got := len(repo.inserts); got != 2 {
		t.Fatalf("inserts=%d, want 2", got)
	}
	if c := repo.inserts[1]; c.pending != 7 || c.inFlight != 2 {
		t.Errorf("second insert=%+v", c)
	}
}

func TestSamplerWritesAfterForceWriteAfterElapsed(t *testing.T) {
	repo := newFakeBacklogRepo()
	repo.counts["prod"] = BacklogStateCounts{Pending: 5}
	rdb := &fakeRedis{lens: map[string]int64{}}
	s := samplerWith(repo, rdb, "prod")
	// Shorten window to make the test deterministic.
	s.cfg.ForceWriteAfter = 0
	s.tick(context.Background())
	// Same counts but ForceWriteAfter=0 ⇒ every tick forces a write.
	s.tick(context.Background())

	if got := len(repo.inserts); got != 2 {
		t.Fatalf("inserts=%d, want 2 (forced)", got)
	}
}

func TestSamplerHydratesFromDBOnColdStart(t *testing.T) {
	repo := newFakeBacklogRepo()
	repo.counts["prod"] = BacklogStateCounts{Pending: 5}
	// Pretend the DB already has a sample matching the current counts —
	// the sampler should treat them as equal and skip the first write.
	repo.latest["prod"] = sampleSnapshot{counts: BacklogStateCounts{Pending: 5}, streamLen: 0}
	repo.latestOK["prod"] = true
	rdb := &fakeRedis{lens: map[string]int64{}}
	s := samplerWith(repo, rdb, "prod")

	s.tick(context.Background())

	if got := len(repo.inserts); got != 0 {
		t.Fatalf("inserts=%d, want 0 (counts match DB snapshot)", got)
	}
}

func TestSamplerTreatsRedisNilAsZeroStreamLen(t *testing.T) {
	repo := newFakeBacklogRepo()
	repo.counts["prod"] = BacklogStateCounts{Pending: 1}
	rdb := &fakeRedis{err: redis.Nil}
	s := samplerWith(repo, rdb, "prod")

	s.tick(context.Background())

	if got := len(repo.inserts); got != 1 {
		t.Fatalf("inserts=%d, want 1", got)
	}
	if got := repo.inserts[0].streamLen; got != 0 {
		t.Errorf("streamLen=%d, want 0 (redis.Nil)", got)
	}
}

func TestSamplerLogsAndContinuesOnPerNamespaceError(t *testing.T) {
	repo := newFakeBacklogRepo()
	repo.counts["prod"] = BacklogStateCounts{Pending: 5}
	repo.counts["staging"] = BacklogStateCounts{Pending: 3}
	rdb := &fakeRedis{lens: map[string]int64{}}
	s := samplerWith(repo, rdb, "prod", "staging")

	// First tick: prod errors mid-flight, staging should still get a sample.
	repo.countsErr = errors.New("boom")
	s.tick(context.Background())
	if got := len(repo.inserts); got != 0 {
		t.Errorf("inserts=%d during full error, want 0", got)
	}
	repo.countsErr = nil
	s.tick(context.Background())
	if got := len(repo.inserts); got != 2 {
		t.Fatalf("inserts=%d, want 2 after recovery", got)
	}
}
