package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	qdrantpb "github.com/qdrant/go-client/qdrant"
	goredis "github.com/redis/go-redis/v9"

	"github.com/jarviisha/codohue/internal/config"
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
	withEmbedderTestHooks(t)
	loadConfigFn = func() (*config.EmbedderConfig, error) {
		return nil, errors.New("config failed")
	}
	if err := run(); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRun_NewPoolError(t *testing.T) {
	withEmbedderTestHooks(t)
	loadConfigFn = func() (*config.EmbedderConfig, error) {
		return validEmbedderConfig(), nil
	}
	newPoolFn = func(_ context.Context, _ string) (*pgxpool.Pool, error) {
		return nil, errors.New("db failed")
	}
	if err := run(); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRun_NewRedisError(t *testing.T) {
	withEmbedderTestHooks(t)
	loadConfigFn = func() (*config.EmbedderConfig, error) {
		return validEmbedderConfig(), nil
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
	withEmbedderTestHooks(t)
	loadConfigFn = func() (*config.EmbedderConfig, error) {
		return validEmbedderConfig(), nil
	}
	newPoolFn = func(_ context.Context, _ string) (*pgxpool.Pool, error) {
		return &pgxpool.Pool{}, nil
	}
	newRedisFn = func(_ string) (*goredis.Client, error) {
		return goredis.NewClient(&goredis.Options{}), nil
	}
	newQdrantFn = func(_ string, _ int) (*qdrantpb.Client, error) {
		return nil, errors.New("qdrant failed")
	}
	if err := run(); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func validEmbedderConfig() *config.EmbedderConfig {
	return &config.EmbedderConfig{
		DatabaseURL:            "postgres://db",
		RedisURL:               "redis://localhost:6379",
		QdrantHost:             "localhost",
		QdrantPort:             6334,
		LogFormat:              "text",
		CatalogMaxContentBytes: 32768,
		EmbedMaxAttempts:       5,
		HealthPort:             "0", // OS-assigned port for any test that does start the server
	}
}

func withEmbedderTestHooks(t *testing.T) {
	t.Helper()
	origLoad := loadConfigFn
	origPool := newPoolFn
	origRedis := newRedisFn
	origQdrant := newQdrantFn
	origRegister := registerMetricsFn
	origNotify := signalNotifyFn
	origClosePool := closePoolFn
	origCloseRedis := closeRedisFn
	origHostname := hostnameFn
	t.Cleanup(func() {
		loadConfigFn = origLoad
		newPoolFn = origPool
		newRedisFn = origRedis
		newQdrantFn = origQdrant
		registerMetricsFn = origRegister
		signalNotifyFn = origNotify
		closePoolFn = origClosePool
		closeRedisFn = origCloseRedis
		hostnameFn = origHostname
	})
	closePoolFn = func(_ *pgxpool.Pool) {}
	closeRedisFn = func(_ *goredis.Client) error { return nil }
	registerMetricsFn = func() {} // tests don't exercise metric registration
}
