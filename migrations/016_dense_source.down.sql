ALTER TABLE namespace_configs
    DROP CONSTRAINT IF EXISTS dense_source_chk;

ALTER TABLE namespace_configs
    DROP COLUMN IF EXISTS dense_source;
