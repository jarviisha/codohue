CREATE TABLE catalog_items (
    id               BIGSERIAL   PRIMARY KEY,
    namespace        TEXT        NOT NULL,
    object_id        TEXT        NOT NULL,
    content          TEXT        NOT NULL,
    content_hash     BYTEA       NOT NULL,
    metadata         JSONB       NOT NULL DEFAULT '{}'::jsonb,
    state            TEXT        NOT NULL DEFAULT 'pending',
    strategy_id      TEXT,
    strategy_version TEXT,
    embedded_at      TIMESTAMPTZ,
    attempt_count    INTEGER     NOT NULL DEFAULT 0,
    last_error       TEXT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (namespace, object_id)
);

CREATE INDEX idx_catalog_items_ns_state
    ON catalog_items (namespace, state);

CREATE INDEX idx_catalog_items_ns_strategy_ver
    ON catalog_items (namespace, strategy_version);

CREATE INDEX idx_catalog_items_state_attempt
    ON catalog_items (state, attempt_count)
    WHERE state IN ('pending', 'failed');

CREATE INDEX idx_catalog_items_updated_at
    ON catalog_items (updated_at DESC);
