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
	if got := trendingKey("ns", 1); got != "trending:ns" {
		t.Fatalf("generation 1: got %q", got)
	}
	if got := trendingKey("ns", 0); got != "trending:ns" {
		t.Fatalf("clamped generation: got %q", got)
	}
	if got := trendingKey("ns", 2); got != "trending:ns:g2" {
		t.Fatalf("generation 2: got %q", got)
	}
}

// Writer and reader must address the same key for a generation-qualified
// namespace — a divergence would serve a recreated namespace the previous
// incarnation's results.
func TestTrending_WriterAndReaderAgreeAtGeneration2(t *testing.T) {
	pipe := &fakePipeline{}
	origPipe := newPipelineFn
	origZRev := zRevRangeWithScoresFn
	t.Cleanup(func() { newPipelineFn = origPipe; zRevRangeWithScoresFn = origZRev })
	newPipelineFn = func(_ *goredis.Client) trendingPipeline { return pipe }
	var readKey string
	zRevRangeWithScoresFn = func(_ context.Context, _ *goredis.Client, key string, _, _ int64) ([]goredis.Z, error) {
		readKey = key
		return nil, nil
	}

	if err := StoreTrending(context.Background(), nil, "ns", 2, map[string]float64{"obj-1": 1}, time.Minute); err != nil {
		t.Fatalf("store: %v", err)
	}
	if _, err := GetTrending(context.Background(), nil, "ns", 2, 0, 10); err != nil {
		t.Fatalf("get: %v", err)
	}
	if pipe.zaddKey == "" || pipe.zaddKey != readKey {
		t.Fatalf("writer wrote %q, reader read %q", pipe.zaddKey, readKey)
	}
	if readKey != "trending:ns:g2" {
		t.Fatalf("generation 2 key: got %q", readKey)
	}
}

func TestStoreTrending_EmptyScoresClearsStaleKey(t *testing.T) {
	pipe := &fakePipeline{}
	orig := newPipelineFn
	t.Cleanup(func() { newPipelineFn = orig })
	newPipelineFn = func(_ *goredis.Client) trendingPipeline {
		return pipe
	}

	if err := StoreTrending(context.Background(), nil, "ns", 1, nil, time.Minute); err != nil {
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

	err := StoreTrending(context.Background(), nil, "ns", 1, map[string]float64{"obj-1": 3.5, "obj-2": 1.2}, 2*time.Minute)
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

	if err := StoreTrending(context.Background(), nil, "ns", 1, map[string]float64{"obj-1": 3.5}, time.Minute); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetTrending_ZeroLimitReturnsNil(t *testing.T) {
	entries, err := GetTrending(context.Background(), nil, "ns", 1, 0, 0)
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

	entries, err := GetTrending(context.Background(), nil, "ns", 1, 1, 2)
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

	if _, err := GetTrending(context.Background(), nil, "ns", 1, 0, 2); err == nil {
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
