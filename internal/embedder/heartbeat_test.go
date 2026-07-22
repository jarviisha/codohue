package embedder

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type fakeHeartbeatWriter struct {
	key   string
	value string
	ttl   time.Duration
	calls int
	err   error
}

func (f *fakeHeartbeatWriter) Set(_ context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd {
	f.calls++
	f.key = key
	if s, ok := value.(string); ok {
		f.value = s
	}
	f.ttl = expiration
	cmd := redis.NewStatusCmd(context.Background())
	if f.err != nil {
		cmd.SetErr(f.err)
	}
	return cmd
}

func TestHeartbeatBeat_StampsKeyWithTTL(t *testing.T) {
	w := &fakeHeartbeatWriter{}
	hb := newHeartbeatWithDeps(w, "replica-a")
	hb.clock = func() time.Time { return time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC) }

	hb.beat(context.Background())

	if w.key != HeartbeatKey {
		t.Errorf("key: got %q, want %q", w.key, HeartbeatKey)
	}
	if w.value != "2026-07-22T10:00:00Z|replica-a" {
		t.Errorf("value: got %q", w.value)
	}
	if w.ttl != heartbeatTTL {
		t.Errorf("ttl: got %v, want %v", w.ttl, heartbeatTTL)
	}
	if w.ttl <= hb.interval {
		t.Error("ttl must exceed the beat interval or a healthy embedder looks dead")
	}
}

func TestHeartbeatBeat_ToleratesRedisError(t *testing.T) {
	w := &fakeHeartbeatWriter{err: errors.New("redis down")}
	hb := newHeartbeatWithDeps(w, "replica-a")
	hb.beat(context.Background()) // must not panic
	if w.calls != 1 {
		t.Fatalf("expected one write attempt, got %d", w.calls)
	}
}

func TestHeartbeatRun_StopsOnContextCancel(t *testing.T) {
	w := &fakeHeartbeatWriter{}
	hb := newHeartbeatWithDeps(w, "")
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		hb.Run(ctx)
	}()
	cancel()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("heartbeat did not stop on context cancel")
	}
	if w.calls == 0 {
		t.Error("expected an immediate first beat before the ticker")
	}
}
