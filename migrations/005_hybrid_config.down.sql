ALTER TABLE namespace_configs
    DROP COLUMN IF EXISTS api_key_hash,
    DROP COLUMN IF EXISTS alpha,
    DROP COLUMN IF EXISTS dense_strategy,
    DROP COLUMN IF EXISTS embedding_dim,
    DROP COLUMN IF EXISTS dense_distance,
    DROP COLUMN IF EXISTS trending_window,
    DROP COLUMN IF EXISTS trending_ttl,
    DROP COLUMN IF EXISTS lambda_trending;
