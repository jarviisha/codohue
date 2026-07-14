-- Unify dense_strategy + catalog_enabled into a single dense_source enum.
-- Phase 1 of the rollout: ADD the column and backfill it; the legacy
-- dense_strategy / catalog_enabled columns are kept for the dual-write window
-- and dropped later in 017 once every reader is on dense_source.
ALTER TABLE namespace_configs
    ADD COLUMN dense_source TEXT NOT NULL DEFAULT 'item2vec';

-- catalog wins: it is the real producer of object dense vectors. The old
-- invariant guaranteed catalog_enabled=TRUE only with dense_strategy in
-- {byoe, disabled}, so no item2vec/svd selection is ever discarded here.
UPDATE namespace_configs
SET dense_source = CASE
    WHEN catalog_enabled THEN 'catalog'
    WHEN dense_strategy IN ('disabled', 'item2vec', 'svd', 'byoe') THEN dense_strategy
    ELSE 'disabled'
END;

ALTER TABLE namespace_configs
    ADD CONSTRAINT dense_source_chk
    CHECK (dense_source IN ('disabled', 'item2vec', 'svd', 'byoe', 'catalog'));
