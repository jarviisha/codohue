package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	parseConfigFn   = pgxpool.ParseConfig
	newWithConfigFn = pgxpool.NewWithConfig
	pingPoolFn      = func(ctx context.Context, pool *pgxpool.Pool) error {
		return pool.Ping(ctx)
	}
)

// NewPool creates and validates a PostgreSQL connection pool.
func NewPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	cfg, err := parseConfigFn(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}

	pool, err := newWithConfigFn(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pingPoolFn(ctx, pool); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return pool, nil
}
