package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNewPool_ParseError(t *testing.T) {
	origParse := parseConfigFn
	t.Cleanup(func() { parseConfigFn = origParse })
	parseConfigFn = func(_ string) (*pgxpool.Config, error) {
		return nil, errors.New("parse failed")
	}

	if _, err := NewPool(context.Background(), "postgres://db"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestNewPool_CreateError(t *testing.T) {
	origParse := parseConfigFn
	origNew := newWithConfigFn
	t.Cleanup(func() {
		parseConfigFn = origParse
		newWithConfigFn = origNew
	})
	parseConfigFn = func(_ string) (*pgxpool.Config, error) {
		return &pgxpool.Config{}, nil
	}
	newWithConfigFn = func(_ context.Context, _ *pgxpool.Config) (*pgxpool.Pool, error) {
		return nil, errors.New("create failed")
	}

	if _, err := NewPool(context.Background(), "postgres://db"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestNewPool_PingError(t *testing.T) {
	origParse := parseConfigFn
	origNew := newWithConfigFn
	origPing := pingPoolFn
	t.Cleanup(func() {
		parseConfigFn = origParse
		newWithConfigFn = origNew
		pingPoolFn = origPing
	})
	parseConfigFn = func(_ string) (*pgxpool.Config, error) {
		return &pgxpool.Config{}, nil
	}
	newWithConfigFn = func(_ context.Context, _ *pgxpool.Config) (*pgxpool.Pool, error) {
		return &pgxpool.Pool{}, nil
	}
	pingPoolFn = func(_ context.Context, _ *pgxpool.Pool) error {
		return errors.New("ping failed")
	}

	if _, err := NewPool(context.Background(), "postgres://db"); err == nil {
		t.Fatal("expected error, got nil")
	}
}
