-- Phase 2 of the dense_source rollout (016 added + backfilled the column):
-- drop the legacy dense_strategy / catalog_enabled pair now that no reader
-- or writer references them. dense_source is the single producer enum.
ALTER TABLE namespace_configs
    DROP COLUMN dense_strategy,
    DROP COLUMN catalog_enabled;
