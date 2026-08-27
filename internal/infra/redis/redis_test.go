package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type fakePipeline struct {
	delKey      string
	zaddKey     string
	zaddMembers []goredis.Z
	expireKey   string
	expireTTL   time.Duration
	execErr     error
}

func (f *fakePipeline) Del(_ context.Context, keys ...string) *goredis.IntCmd {
	if len(keys) > 0 {
		f.delKey = keys[0]
	}
	return goredis.NewIntCmd(context.Background())
}

func (f *fakePipeline) ZAdd(_ context.Context, key string, members ...goredis.Z) *goredis.IntCmd {
	f.zaddKey = key
	f.zaddMembers = append([]goredis.Z(nil), members...)
	return goredis.NewIntCmd(context.Background())
}

func (f *fakePipeline) Expire(_ context.Context, key string, expiration time.Duration) *goredis.BoolCmd {
	f.expireKey = key
	f.expireTTL = expiration
	return goredis.NewBoolCmd(context.Background())
}

func (f *fakePipeline) Exec(_ context.Context) ([]goredis.Cmder, error) {
	return nil, f.execErr
}

func TestTrendingKey(t *testing.T) {
	if got := trendingKey("ns"); got != "trending:ns" {
		t.Fatalf("got %q", got)
	}
}

func TestStoreTrending_EmptyScoresClearsStaleKey(t *testing.T) {
	pipe := &fakePipeline{}
	orig := newPipelineFn
	t.Cleanup(func() { newPipelineFn = orig })
	newPipelineFn = func(_ *goredis.Client) trendingPipeline {
		return pipe
	}

	if err := StoreTrending(context.Background(), nil, "ns", nil, time.Minute); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pipe.delKey != "trending:ns" || pipe.zaddKey != "" || pipe.expireKey != "" {
		t.Fatalf("empty scores must only delete stale key: %+v", pipe)
	}
}

func TestStoreTrending_PipelinesCommands(t *testing.T) {
	pipe := &fakePipeline{}
	orig := newPipelineFn
	t.Cleanup(func() { newPipelineFn = orig })
	newPipelineFn = func(_ *goredis.Client) trendingPipeline { return pipe }

	err := StoreTrending(context.Background(), nil, "ns", map[string]float64{"obj-1": 3.5, "obj-2": 1.2}, 2*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pipe.delKey != "trending:ns" || pipe.zaddKey != "trending:ns" || pipe.expireKey != "trending:ns" {
		t.Fatalf("unexpected keys: del=%s zadd=%s expire=%s", pipe.delKey, pipe.zaddKey, pipe.expireKey)
	}
	if pipe.expireTTL != 2*time.Minute {
		t.Fatalf("ttl: got %v", pipe.expireTTL)
	}
	if len(pipe.zaddMembers) != 2 {
		t.Fatalf("expected 2 members, got %d", len(pipe.zaddMembers))
	}
}

func TestStoreTrending_ExecError(t *testing.T) {
	orig := newPipelineFn
	t.Cleanup(func() { newPipelineFn = orig })
	newPipelineFn = func(_ *goredis.Client) trendingPipeline { return &fakePipeline{execErr: errors.New("exec failed")} }

	if err := StoreTrending(context.Background(), nil, "ns", map[string]float64{"obj-1": 3.5}, time.Minute); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetTrending_ZeroLimitReturnsNil(t *testing.T) {
	entries, err := GetTrending(context.Background(), nil, "ns", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entries != nil {
		t.Fatalf("expected nil entries, got %+v", entries)
	}
}

func TestGetTrending_ReturnsEntriesAndSkipsNonStringMembers(t *testing.T) {
	orig := zRevRangeWithScoresFn
	t.Cleanup(func() { zRevRangeWithScoresFn = orig })
	zRevRangeWithScoresFn = func(_ context.Context, _ *goredis.Client, key string, start, stop int64) ([]goredis.Z, error) {
		if key != "trending:ns" || start != 1 || stop != 2 {
			t.Fatalf("unexpected args key=%s start=%d stop=%d", key, start, stop)
		}
		return []goredis.Z{
			{Member: "obj-1", Score: 9.5},
			{Member: 123, Score: 7.1},
		}, nil
	}

	entries, err := GetTrending(context.Background(), nil, "ns", 1, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 1 || entries[0].ObjectID != "obj-1" || entries[0].Score != 9.5 {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}

func TestGetTrending_QueryError(t *testing.T) {
	orig := zRevRangeWithScoresFn
	t.Cleanup(func() { zRevRangeWithScoresFn = orig })
	zRevRangeWithScoresFn = func(_ context.Context, _ *goredis.Client, _ string, _, _ int64) ([]goredis.Z, error) {
		return nil, errors.New("redis failed")
	}

	if _, err := GetTrending(context.Background(), nil, "ns", 0, 2); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestNewClient_ParseError(t *testing.T) {
	if _, err := NewClient("://bad-url"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestNewClient_PingError(t *testing.T) {
	origPing := pingClientFn
	t.Cleanup(func() { pingClientFn = origPing })
	pingClientFn = func(_ context.Context, _ *goredis.Client) error {
		return errors.New("ping failed")
	}

	if _, err := NewClient("redis://localhost:6379"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestNewClient_Success(t *testing.T) {
	original := pingClientFn
	t.Cleanup(func() { pingClientFn = original })
	pingClientFn = func(context.Context, *goredis.Client) error { return nil }

	client, err := NewClient("redis://localhost:6379")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	if client == nil {
		t.Fatal("NewClient returned no client")
	}
	t.Cleanup(func() { _ = client.Close() })
}

// Generation 1 keeps the original unqualified key, so an upgrade moves
// nothing. A generation below 1 is not a real lifecycle value; clamping keeps
// a bad caller from inventing a key no writer will ever produce.
func TestTrendingKeyForGeneration_ClampsBelowOne(t *testing.T) {
	base := trendingKeyForGeneration("ns", 1)
	for _, generation := range []int64{0, -1} {
		if got := trendingKeyForGeneration("ns", generation); got != base {
			t.Errorf("generation %d gave key %q, want %q", generation, got, base)
		}
	}
	if g2 := trendingKeyForGeneration("ns", 2); g2 == base {
		t.Error("generation 2 shares generation 1's key; a deleted incarnation would stay visible")
	}
}
