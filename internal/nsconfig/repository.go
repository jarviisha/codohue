package nsconfig

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jarviisha/codohue/internal/core/namespace"
)

type rowScanner interface {
	Scan(dest ...any) error
}

// Repository performs CRUD operations on namespace_configs in PostgreSQL.
type Repository struct {
	db         *pgxpool.Pool
	execFn     func(ctx context.Context, sql string, args ...any) error
	queryRowFn func(ctx context.Context, sql string, args ...any) rowScanner
}

// NewRepository creates a new Repository with the given PostgreSQL connection pool.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{
		db: db,
		execFn: func(ctx context.Context, sql string, args ...any) error {
			_, err := db.Exec(ctx, sql, args...)
			if err != nil {
				return fmt.Errorf("exec namespace config statement: %w", err)
			}
			return nil
		},
		queryRowFn: func(ctx context.Context, sql string, args ...any) rowScanner {
			return db.QueryRow(ctx, sql, args...)
		},
	}
}

// Upsert creates or updates the configuration for a namespace.
// api_key_hash is intentionally excluded from the ON CONFLICT UPDATE clause
// so that an existing key is never overwritten by a config update.
func (r *Repository) Upsert(ctx context.Context, ns string, req *UpsertRequest) (*namespace.Config, error) {
	weightsJSON, err := json.Marshal(req.ActionWeights)
	if err != nil {
		return nil, fmt.Errorf("marshal action weights: %w", err)
	}

	var cfg namespace.Config
	var weightsRaw []byte

	err = r.queryRowFn(ctx, `
		INSERT INTO namespace_configs (
			namespace, action_weights, time_decay_factor, gamma, max_results, seen_items_days,
			alpha, dense_strategy, embedding_dim, dense_distance,
			trending_window, trending_ttl, lambda_trending,
			updated_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW())
		ON CONFLICT (namespace) DO UPDATE
		  SET action_weights    = EXCLUDED.action_weights,
		      time_decay_factor = EXCLUDED.time_decay_factor,
		      gamma             = EXCLUDED.gamma,
		      max_results       = EXCLUDED.max_results,
		      seen_items_days   = EXCLUDED.seen_items_days,
		      alpha             = EXCLUDED.alpha,
		      dense_strategy    = EXCLUDED.dense_strategy,
		      embedding_dim     = EXCLUDED.embedding_dim,
		      dense_distance    = EXCLUDED.dense_distance,
		      trending_window   = EXCLUDED.trending_window,
		      trending_ttl      = EXCLUDED.trending_ttl,
		      lambda_trending   = EXCLUDED.lambda_trending,
		      updated_at        = NOW()
		RETURNING
			namespace, action_weights, time_decay_factor, gamma, max_results, seen_items_days,
			COALESCE(api_key_hash, ''),
			alpha, dense_strategy, embedding_dim, dense_distance,
			trending_window, trending_ttl, lambda_trending,
			created_at, updated_at`,
		ns, weightsJSON, req.Lambda, req.Gamma, req.MaxResults, req.SeenItemsDays,
		req.Alpha, req.DenseStrategy, req.EmbeddingDim, req.DenseDistance,
		req.TrendingWindow, req.TrendingTTL, req.LambdaTrending,
	).Scan(
		&cfg.Namespace, &weightsRaw, &cfg.Lambda, &cfg.Gamma, &cfg.MaxResults, &cfg.SeenItemsDays,
		&cfg.APIKeyHash,
		&cfg.Alpha, &cfg.DenseStrategy, &cfg.EmbeddingDim, &cfg.DenseDistance,
		&cfg.TrendingWindow, &cfg.TrendingTTL, &cfg.LambdaTrending,
		&cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert namespace config: %w", err)
	}

	if err := json.Unmarshal(weightsRaw, &cfg.ActionWeights); err != nil {
		return nil, fmt.Errorf("unmarshal action weights: %w", err)
	}

	return &cfg, nil
}

// SetAPIKeyHash stores the bcrypt hash for the namespace. It is a no-op if the
// namespace already has a hash (first-write-wins, matching the INSERT-then-check
// pattern in Service.Upsert).
func (r *Repository) SetAPIKeyHash(ctx context.Context, namespace, hash string) error {
	err := r.execFn(ctx, `
		UPDATE namespace_configs
		SET api_key_hash = $2
		WHERE namespace = $1 AND api_key_hash IS NULL`,
		namespace, hash,
	)
	if err != nil {
		return fmt.Errorf("set api key hash: %w", err)
	}
	return nil
}

// Get returns the configuration for a namespace, or nil if it does not exist.
func (r *Repository) Get(ctx context.Context, ns string) (*namespace.Config, error) {
	var cfg namespace.Config
	var weightsRaw []byte

	err := r.queryRowFn(ctx, `
		SELECT
			namespace, action_weights, time_decay_factor, gamma, max_results, seen_items_days,
			COALESCE(api_key_hash, ''),
			alpha, dense_strategy, embedding_dim, dense_distance,
			trending_window, trending_ttl, lambda_trending,
			created_at, updated_at
		FROM namespace_configs
		WHERE namespace = $1`,
		ns,
	).Scan(
		&cfg.Namespace, &weightsRaw, &cfg.Lambda, &cfg.Gamma, &cfg.MaxResults, &cfg.SeenItemsDays,
		&cfg.APIKeyHash,
		&cfg.Alpha, &cfg.DenseStrategy, &cfg.EmbeddingDim, &cfg.DenseDistance,
		&cfg.TrendingWindow, &cfg.TrendingTTL, &cfg.LambdaTrending,
		&cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get namespace config: %w", err)
	}

	if err := json.Unmarshal(weightsRaw, &cfg.ActionWeights); err != nil {
		return nil, fmt.Errorf("unmarshal action weights: %w", err)
	}

	return &cfg, nil
}
