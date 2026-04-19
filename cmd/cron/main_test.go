package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jarviisha/codohue/internal/config"
	qdrantpb "github.com/qdrant/go-client/qdrant"
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
