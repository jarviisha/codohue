DROP INDEX IF EXISTS idx_catalog_items_ns_updated_id;
ALTER TABLE objects DROP CONSTRAINT IF EXISTS objects_namespace_fk;
ALTER TABLE catalog_backlog_samples DROP CONSTRAINT IF EXISTS catalog_backlog_samples_namespace_fk;
ALTER TABLE catalog_items DROP CONSTRAINT IF EXISTS catalog_items_namespace_fk;
ALTER TABLE batch_run_logs DROP CONSTRAINT IF EXISTS batch_run_logs_namespace_fk;
ALTER TABLE id_mappings DROP CONSTRAINT IF EXISTS id_mappings_namespace_fk;
ALTER TABLE events DROP CONSTRAINT IF EXISTS events_namespace_fk;
