-- Recreate the legacy pair with the same defaults as migrations 005/011 and
-- backfill from dense_source. Lossy by design for one case: a pre-016
-- namespace that had dense_strategy='byoe' alongside catalog_enabled=TRUE
-- comes back as 'disabled' — catalog is authoritative and object BYOE writes
-- are rejected in catalog mode anyway.
ALTER TABLE namespace_configs
    ADD COLUMN dense_strategy  TEXT    NOT NULL DEFAULT 'item2vec',
    ADD COLUMN catalog_enabled BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE namespace_configs
SET dense_strategy  = CASE WHEN dense_source = 'catalog' THEN 'disabled' ELSE dense_source END,
    catalog_enabled = (dense_source = 'catalog');
