ALTER TABLE namespace_configs DROP CONSTRAINT IF EXISTS namespace_configs_lifecycle_fk;
ALTER TABLE namespace_configs DROP COLUMN IF EXISTS generation;
DROP TABLE IF EXISTS system_lifecycle;
DROP TABLE IF EXISTS namespace_lifecycles;
