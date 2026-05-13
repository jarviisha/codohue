ALTER TABLE namespace_configs
    ADD COLUMN catalog_enabled           BOOLEAN  NOT NULL DEFAULT FALSE,
    ADD COLUMN catalog_strategy_id       TEXT,
    ADD COLUMN catalog_strategy_version  TEXT,
    ADD COLUMN catalog_strategy_params   JSONB    NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN catalog_max_attempts      INTEGER  NOT NULL DEFAULT 5,
    ADD COLUMN catalog_max_content_bytes INTEGER  NOT NULL DEFAULT 32768;
