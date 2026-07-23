package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jarviisha/codohue/internal/config"
	qdrantpb "github.com/qdrant/go-client/qdrant"
	goredis "github.com/redis/go-redis/v9"
)

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
	withCronTestHooks(t)
	loadConfigFn = func() (*config.AppConfig, error) {
		return nil, errors.New("config failed")
	}

	if err := run(); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRun_NewPoolError(t *testing.T) {
	withCronTestHooks(t)
	loadConfigFn = func() (*config.AppConfig, error) {
		return &config.AppConfig{DatabaseURL: "postgres://db", RedisURL: "redis://localhost:6379", QdrantHost: "localhost", QdrantPort: 6334, BatchIntervalMinutes: 5}, nil
	}
	newPoolFn = func(_ context.Context, _ string) (*pgxpool.Pool, error) {
		return nil, errors.New("db failed")
	}

	if err := run(); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRun_NewQdrantError(t *testing.T) {
	withCronTestHooks(t)
	loadConfigFn = func() (*config.AppConfig, error) {
		return &config.AppConfig{DatabaseURL: "postgres://db", RedisURL: "redis://localhost:6379", QdrantHost: "localhost", QdrantPort: 6334, BatchIntervalMinutes: 5}, nil
	}
	newPoolFn = func(_ context.Context, _ string) (*pgxpool.Pool, error) {
		return &pgxpool.Pool{}, nil
	}
	newQdrantFn = func(_ string, _ int) (*qdrantpb.Client, error) {
		return nil, errors.New("qdrant failed")
	}

	if err := run(); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func withCronTestHooks(t *testing.T) {
	t.Helper()
	origLoad := loadConfigFn
	origPool := newPoolFn
	origQdrant := newQdrantFn
	origRedis := newRedisFn
	origNotify := signalNotifyFn
	origClosePool := closePoolFn
	t.Cleanup(func() {
		loadConfigFn = origLoad
		newPoolFn = origPool
		newQdrantFn = origQdrant
		newRedisFn = origRedis
		signalNotifyFn = origNotify
		closePoolFn = origClosePool
	})
	closePoolFn = func(_ *pgxpool.Pool) {}
}

func TestDrainDone_AllClosed(t *testing.T) {
	a, b := make(chan struct{}), make(chan struct{})
	close(a)
	close(b)
	never := make(chan time.Time)
	if !drainDone([]<-chan struct{}{a, b}, never) {
		t.Fatal("all-closed channels must drain cleanly (true)")
	}
}

func TestDrainDone_Timeout(t *testing.T) {
	a := make(chan struct{}) // never closes
	fired := make(chan time.Time, 1)
	fired <- time.Now()
	if drainDone([]<-chan struct{}{a}, fired) {
		t.Fatal("a hung goroutine must make drainDone return false on timeout")
	}
}

func TestDrainDone_Empty(t *testing.T) {
	never := make(chan time.Time)
	if !drainDone(nil, never) {
		t.Fatal("no channels to wait on drains cleanly")
	}
}

// TestRun_HappyPathStartupShutdown drives run() through full wiring and a
// clean shutdown against a real (migrated, empty) Postgres. It covers the
// observer wiring, background-goroutine spawns, and the drain path — the
// startup/shutdown lifecycle no other test reaches. Gated on DATABASE_URL, so
// it runs in CI (which provisions Postgres) and skips locally without one.
func TestRun_HappyPathStartupShutdown(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("requires DATABASE_URL (a migrated Postgres)")
	}
	withCronTestHooks(t)

	loadConfigFn = func() (*config.AppConfig, error) {
		return &config.AppConfig{
			DatabaseURL:          dsn,
			RedisURL:             "redis://localhost:6379",
			QdrantHost:           "localhost",
			QdrantPort:           6334,
			BatchIntervalMinutes: 1,
			RetentionInterval:    time.Hour,
		}, nil
	}
	newPoolFn = pgxpool.New
	// nil qdrant/redis are fine on an empty DB: no active namespaces means
	// no vector-store calls, and a nil redis skips the trending phase.
	newQdrantFn = func(_ string, _ int) (*qdrantpb.Client, error) { return nil, nil }
	newRedisFn = func(_ string) (*goredis.Client, error) { return nil, errors.New("no redis in this test") }
	// Fire the shutdown signal immediately so run() proceeds straight to the
	// cancel + drain path after wiring completes.
	signalNotifyFn = func(c chan<- os.Signal, _ ...os.Signal) {
		go func() { c <- os.Interrupt }()
	}

	done := make(chan error, 1)
	go func() { done <- run() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run() returned error: %v", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("run() did not shut down within 20s")
	}
}
