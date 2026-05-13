ALTER TABLE namespace_configs
    DROP COLUMN IF EXISTS catalog_enabled,
    DROP COLUMN IF EXISTS catalog_strategy_id,
    DROP COLUMN IF EXISTS catalog_strategy_version,
    DROP COLUMN IF EXISTS catalog_strategy_params,
    DROP COLUMN IF EXISTS catalog_max_attempts,
    DROP COLUMN IF EXISTS catalog_max_content_bytes;
