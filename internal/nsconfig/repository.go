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
//
// Catalog auto-embedding columns (catalog_*) are also excluded from the
// UPDATE clause: they are owned by the separate UpsertCatalogConfig path.
// On INSERT they fall back to the column defaults declared in migration 011.
func (r *Repository) Upsert(ctx context.Context, ns string, req *UpsertRequest) (*namespace.Config, error) {
	weightsJSON, err := json.Marshal(req.ActionWeights)
	if err != nil {
		return nil, fmt.Errorf("marshal action weights: %w", err)
	}

	var cfg namespace.Config
	var weightsRaw []byte
	var paramsRaw []byte

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
			catalog_enabled, COALESCE(catalog_strategy_id, ''), COALESCE(catalog_strategy_version, ''),
			catalog_strategy_params, catalog_max_attempts, catalog_max_content_bytes,
			created_at, updated_at`,
		ns, weightsJSON, req.Lambda, req.Gamma, req.MaxResults, req.SeenItemsDays,
		req.Alpha, req.DenseStrategy, req.EmbeddingDim, req.DenseDistance,
		req.TrendingWindow, req.TrendingTTL, req.LambdaTrending,
	).Scan(
		&cfg.Namespace, &weightsRaw, &cfg.Lambda, &cfg.Gamma, &cfg.MaxResults, &cfg.SeenItemsDays,
		&cfg.APIKeyHash,
		&cfg.Alpha, &cfg.DenseStrategy, &cfg.EmbeddingDim, &cfg.DenseDistance,
		&cfg.TrendingWindow, &cfg.TrendingTTL, &cfg.LambdaTrending,
		&cfg.CatalogEnabled, &cfg.CatalogStrategyID, &cfg.CatalogStrategyVersion,
		&paramsRaw, &cfg.CatalogMaxAttempts, &cfg.CatalogMaxContentBytes,
		&cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert namespace config: %w", err)
	}

	if err := json.Unmarshal(weightsRaw, &cfg.ActionWeights); err != nil {
		return nil, fmt.Errorf("unmarshal action weights: %w", err)
	}
	cfg.CatalogStrategyParams, err = unmarshalCatalogParams(paramsRaw)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

// SetAPIKeyHash stores the bcrypt hash for the namespace. It is a no-op if the
// namespace already has a hash (first-write-wins, matching the INSERT-then-check
// pattern in Service.Upsert).
func (r *Repository) SetAPIKeyHash(ctx context.Context, ns, hash string) error {
	err := r.execFn(ctx, `
		UPDATE namespace_configs
		SET api_key_hash = $2
		WHERE namespace = $1 AND api_key_hash IS NULL`,
		ns, hash,
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
	var paramsRaw []byte

	err := r.queryRowFn(ctx, `
		SELECT
			namespace, action_weights, time_decay_factor, gamma, max_results, seen_items_days,
			COALESCE(api_key_hash, ''),
			alpha, dense_strategy, embedding_dim, dense_distance,
			trending_window, trending_ttl, lambda_trending,
			catalog_enabled, COALESCE(catalog_strategy_id, ''), COALESCE(catalog_strategy_version, ''),
			catalog_strategy_params, catalog_max_attempts, catalog_max_content_bytes,
			created_at, updated_at
		FROM namespace_configs
		WHERE namespace = $1`,
		ns,
	).Scan(
		&cfg.Namespace, &weightsRaw, &cfg.Lambda, &cfg.Gamma, &cfg.MaxResults, &cfg.SeenItemsDays,
		&cfg.APIKeyHash,
		&cfg.Alpha, &cfg.DenseStrategy, &cfg.EmbeddingDim, &cfg.DenseDistance,
		&cfg.TrendingWindow, &cfg.TrendingTTL, &cfg.LambdaTrending,
		&cfg.CatalogEnabled, &cfg.CatalogStrategyID, &cfg.CatalogStrategyVersion,
		&paramsRaw, &cfg.CatalogMaxAttempts, &cfg.CatalogMaxContentBytes,
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
	cfg.CatalogStrategyParams, err = unmarshalCatalogParams(paramsRaw)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

// ListCatalogEnabled returns the configuration of every namespace that
// currently has catalog auto-embedding enabled. Used by the embedder
// binary to discover which per-namespace Redis Streams to consume.
//
// Returns an empty slice (not nil error) when no namespaces are enabled.
// The result order is namespace ASC for stable test output.
func (r *Repository) ListCatalogEnabled(ctx context.Context) ([]*namespace.Config, error) {
	if r.db == nil {
		// Allow unit tests that exercise other methods to leave db nil; this
		// method is only called from the embedder where db is always set.
		return nil, fmt.Errorf("nsconfig: db is nil")
	}
	rows, err := r.db.Query(ctx, `
		SELECT
			namespace, action_weights, time_decay_factor, gamma, max_results, seen_items_days,
			COALESCE(api_key_hash, ''),
			alpha, dense_strategy, embedding_dim, dense_distance,
			trending_window, trending_ttl, lambda_trending,
			catalog_enabled, COALESCE(catalog_strategy_id, ''), COALESCE(catalog_strategy_version, ''),
			catalog_strategy_params, catalog_max_attempts, catalog_max_content_bytes,
			created_at, updated_at
		FROM namespace_configs
		WHERE catalog_enabled = TRUE
		ORDER BY namespace ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list catalog-enabled namespaces: %w", err)
	}
	defer rows.Close()

	out := make([]*namespace.Config, 0, 4)
	for rows.Next() {
		var cfg namespace.Config
		var weightsRaw []byte
		var paramsRaw []byte
		err := rows.Scan(
			&cfg.Namespace, &weightsRaw, &cfg.Lambda, &cfg.Gamma, &cfg.MaxResults, &cfg.SeenItemsDays,
			&cfg.APIKeyHash,
			&cfg.Alpha, &cfg.DenseStrategy, &cfg.EmbeddingDim, &cfg.DenseDistance,
			&cfg.TrendingWindow, &cfg.TrendingTTL, &cfg.LambdaTrending,
			&cfg.CatalogEnabled, &cfg.CatalogStrategyID, &cfg.CatalogStrategyVersion,
			&paramsRaw, &cfg.CatalogMaxAttempts, &cfg.CatalogMaxContentBytes,
			&cfg.CreatedAt, &cfg.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan namespace config row: %w", err)
		}
		if err := json.Unmarshal(weightsRaw, &cfg.ActionWeights); err != nil {
			return nil, fmt.Errorf("unmarshal action weights: %w", err)
		}
		cfg.CatalogStrategyParams, err = unmarshalCatalogParams(paramsRaw)
		if err != nil {
			return nil, err
		}
		out = append(out, &cfg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate namespace configs: %w", err)
	}
	return out, nil
}

// UpsertCatalogConfig writes the catalog-specific columns for an existing
// namespace. The namespace must already exist (call Upsert first); this
// method does not create rows. Caller is responsible for any cross-field
// validation (e.g. dimension match) — the repository only persists.
func (r *Repository) UpsertCatalogConfig(ctx context.Context, ns string, req *UpdateCatalogRequest) (*namespace.Config, error) {
	paramsJSON, err := marshalCatalogParams(req.Params)
	if err != nil {
		return nil, err
	}

	var (
		strategyID, strategyVer any
	)
	if req.Enabled {
		strategyID = req.StrategyID
		strategyVer = req.StrategyVersion
	} else {
		// Persist as NULL when disabled so a future enable starts from a
		// clean slot rather than an inherited identifier.
		strategyID = nil
		strategyVer = nil
	}

	maxAttempts := req.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	maxBytes := req.MaxContentBytes
	if maxBytes <= 0 {
		maxBytes = 32768
	}

	var cfg namespace.Config
	var weightsRaw []byte
	var paramsRaw []byte

	err = r.queryRowFn(ctx, `
		UPDATE namespace_configs
		SET catalog_enabled            = $2,
		    catalog_strategy_id        = $3,
		    catalog_strategy_version   = $4,
		    catalog_strategy_params    = $5,
		    catalog_max_attempts       = $6,
		    catalog_max_content_bytes  = $7,
		    updated_at                 = NOW()
		WHERE namespace = $1
		RETURNING
			namespace, action_weights, time_decay_factor, gamma, max_results, seen_items_days,
			COALESCE(api_key_hash, ''),
			alpha, dense_strategy, embedding_dim, dense_distance,
			trending_window, trending_ttl, lambda_trending,
			catalog_enabled, COALESCE(catalog_strategy_id, ''), COALESCE(catalog_strategy_version, ''),
			catalog_strategy_params, catalog_max_attempts, catalog_max_content_bytes,
			created_at, updated_at`,
		ns, req.Enabled, strategyID, strategyVer, paramsJSON, maxAttempts, maxBytes,
	).Scan(
		&cfg.Namespace, &weightsRaw, &cfg.Lambda, &cfg.Gamma, &cfg.MaxResults, &cfg.SeenItemsDays,
		&cfg.APIKeyHash,
		&cfg.Alpha, &cfg.DenseStrategy, &cfg.EmbeddingDim, &cfg.DenseDistance,
		&cfg.TrendingWindow, &cfg.TrendingTTL, &cfg.LambdaTrending,
		&cfg.CatalogEnabled, &cfg.CatalogStrategyID, &cfg.CatalogStrategyVersion,
		&paramsRaw, &cfg.CatalogMaxAttempts, &cfg.CatalogMaxContentBytes,
		&cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("update catalog config: %w", err)
	}

	if err := json.Unmarshal(weightsRaw, &cfg.ActionWeights); err != nil {
		return nil, fmt.Errorf("unmarshal action weights: %w", err)
	}
	cfg.CatalogStrategyParams, err = unmarshalCatalogParams(paramsRaw)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

func marshalCatalogParams(p map[string]any) ([]byte, error) {
	if p == nil {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal catalog strategy params: %w", err)
	}
	return b, nil
}

func unmarshalCatalogParams(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("unmarshal catalog strategy params: %w", err)
	}
	if len(m) == 0 {
		return nil, nil
	}
	return m, nil
}
