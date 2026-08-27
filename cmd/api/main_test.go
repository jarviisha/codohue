package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jarviisha/codohue/internal/config"
	infraredis "github.com/jarviisha/codohue/internal/infra/redis"
	qdrantpb "github.com/qdrant/go-client/qdrant"
	goredis "github.com/redis/go-redis/v9"
)

type fakeRetentionRunner struct {
	calls chan infraredis.StreamSpec
}

func (f *fakeRetentionRunner) RunOnce(_ context.Context, spec infraredis.StreamSpec) (infraredis.RetentionResult, error) {
	f.calls <- spec
	return infraredis.RetentionResult{}, nil
}

func TestAPIRetentionLoopStartsAndStops(t *testing.T) {
	specs := apiRetentionSpecs()
	if len(specs) != 2 || specs[0].Name != "codohue:events" || specs[1].Name != "codohue:catalog" {
		t.Fatalf("api retention specs = %+v", specs)
	}
	if got := specs[0].ExpectedGroups; len(got) != 1 || got[0] != "codohue-ingest" {
		t.Fatalf("event expected groups = %v", got)
	}
	if got := specs[1].ExpectedGroups; len(got) != 1 || got[0] != "codohue-catalog-ingest" {
		t.Fatalf("catalog expected groups = %v", got)
	}

	runner := &fakeRetentionRunner{calls: make(chan infraredis.StreamSpec, len(specs))}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		infraredis.RunRetentionLoop(ctx, time.Hour, runner, func(context.Context) ([]infraredis.StreamSpec, error) {
			return specs, nil
		})
	}()
	for range specs {
		select {
		case <-runner.calls:
		case <-time.After(time.Second):
			t.Fatal("retention loop did not run initial pass")
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("retention loop did not stop after cancellation")
	}
}

func TestAPIRetentionDryRunTracksKillSwitch(t *testing.T) {
	if !retentionDryRun(false) {
		t.Fatal("disabled retention must inspect in dry-run mode")
	}
	if retentionDryRun(true) {
		t.Fatal("enabled retention must execute exact trimming")
	}
}

func TestPingHandler(t *testing.T) {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/ping", http.NoBody)
	rec := httptest.NewRecorder()

	pingHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("content-type: got %q", rec.Header().Get("Content-Type"))
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestInitLogger(t *testing.T) {
	initLogger("json")
	if _, ok := slog.Default().Handler().(*slog.JSONHandler); !ok {
		t.Fatal("expected JSON handler")
	}

	initLogger("text")
	if _, ok := slog.Default().Handler().(*slog.TextHandler); !ok {
		t.Fatal("expected Text handler")
	}
}

func TestRun_LoadConfigError(t *testing.T) {
	withAPITestHooks(t)
	loadConfigFn = func() (*config.AppConfig, error) {
		return nil, errors.New("config failed")
	}

	if err := run(); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRun_NewPoolError(t *testing.T) {
	withAPITestHooks(t)
	loadConfigFn = func() (*config.AppConfig, error) {
		return &config.AppConfig{DatabaseURL: "postgres://db", RedisURL: "redis://localhost:6379", QdrantHost: "localhost", QdrantPort: 6334, AdminAPIKey: "admin", APIPort: "2001"}, nil
	}
	newPoolFn = func(_ context.Context, _ string) (*pgxpool.Pool, error) {
		return nil, errors.New("db failed")
	}

	if err := run(); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRun_NewRedisError(t *testing.T) {
	withAPITestHooks(t)
	loadConfigFn = func() (*config.AppConfig, error) {
		return &config.AppConfig{DatabaseURL: "postgres://db", RedisURL: "redis://localhost:6379", QdrantHost: "localhost", QdrantPort: 6334, AdminAPIKey: "admin", APIPort: "2001"}, nil
	}
	newPoolFn = func(_ context.Context, _ string) (*pgxpool.Pool, error) {
		return &pgxpool.Pool{}, nil
	}
	newRedisFn = func(_ string) (*goredis.Client, error) {
		return nil, errors.New("redis failed")
	}

	if err := run(); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRun_NewQdrantError(t *testing.T) {
	withAPITestHooks(t)
	loadConfigFn = func() (*config.AppConfig, error) {
		return &config.AppConfig{DatabaseURL: "postgres://db", RedisURL: "redis://localhost:6379", QdrantHost: "localhost", QdrantPort: 6334, AdminAPIKey: "admin", APIPort: "2001"}, nil
	}
	newPoolFn = func(_ context.Context, _ string) (*pgxpool.Pool, error) {
		return &pgxpool.Pool{}, nil
	}
	newRedisFn = func(_ string) (*goredis.Client, error) {
		return &goredis.Client{}, nil
	}
	newQdrantFn = func(_ string, _ int) (*qdrantpb.Client, error) {
		return nil, errors.New("qdrant failed")
	}

	if err := run(); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCheckPostgres(t *testing.T) {
	orig := dbPingFn
	t.Cleanup(func() { dbPingFn = orig })

	dbPingFn = func(context.Context, *pgxpool.Pool) error { return nil }
	if got := checkPostgres(context.Background(), nil); got != "ok" {
		t.Fatalf("expected ok, got %q", got)
	}

	dbPingFn = func(context.Context, *pgxpool.Pool) error { return errors.New("db down") }
	if got := checkPostgres(context.Background(), nil); got != "error: db down" {
		t.Fatalf("unexpected result: %q", got)
	}
}

func TestCheckRedis(t *testing.T) {
	orig := redisPingRawFn
	t.Cleanup(func() { redisPingRawFn = orig })

	redisPingRawFn = func(context.Context, *goredis.Client) error { return nil }
	if got := checkRedis(context.Background(), nil); got != "ok" {
		t.Fatalf("expected ok, got %q", got)
	}

	redisPingRawFn = func(context.Context, *goredis.Client) error { return errors.New("redis down") }
	if got := checkRedis(context.Background(), nil); got != "error: redis down" {
		t.Fatalf("unexpected result: %q", got)
	}
}

func TestCheckQdrant(t *testing.T) {
	orig := qdrantHealthFn
	t.Cleanup(func() { qdrantHealthFn = orig })

	qdrantHealthFn = func(context.Context, *qdrantpb.Client) error { return nil }
	if got := checkQdrant(context.Background(), nil); got != "ok" {
		t.Fatalf("expected ok, got %q", got)
	}

	qdrantHealthFn = func(context.Context, *qdrantpb.Client) error { return errors.New("qdrant down") }
	if got := checkQdrant(context.Background(), nil); got != "error: qdrant down" {
		t.Fatalf("unexpected result: %q", got)
	}
}

func withAPITestHooks(t *testing.T) {
	t.Helper()
	origLoad := loadConfigFn
	origPool := newPoolFn
	origRedis := newRedisFn
	origQdrant := newQdrantFn
	origRegister := registerMetricsFn
	origNotify := signalNotifyFn
	origClosePool := closePoolFn
	origCloseRedis := closeRedisFn
	origDBPing := dbPingFn
	origRedisPing := redisPingRawFn
	origQdrantHealth := qdrantHealthFn
	t.Cleanup(func() {
		loadConfigFn = origLoad
		newPoolFn = origPool
		newRedisFn = origRedis
		newQdrantFn = origQdrant
		registerMetricsFn = origRegister
		signalNotifyFn = origNotify
		closePoolFn = origClosePool
		closeRedisFn = origCloseRedis
		dbPingFn = origDBPing
		redisPingRawFn = origRedisPing
		qdrantHealthFn = origQdrantHealth
	})
	registerMetricsFn = func() {}
	signalNotifyFn = func(_ chan<- os.Signal, _ ...os.Signal) {}
	closePoolFn = func(_ *pgxpool.Pool) {}
	closeRedisFn = func(_ *goredis.Client) error { return nil }
}

func TestResolveConsumerName(t *testing.T) {
	if got := resolveConsumerName("replica-a", func() (string, error) { return "host", nil }); got != "replica-a" {
		t.Errorf("configured name wins: got %q", got)
	}
	if got := resolveConsumerName("", func() (string, error) { return "host-x", nil }); got != "host-x" {
		t.Errorf("hostname fallback: got %q", got)
	}
	if got := resolveConsumerName("", func() (string, error) { return "", errors.New("no hostname") }); got != "" {
		t.Errorf("hostname error falls back to empty: got %q", got)
	}
}
