DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM events child LEFT JOIN namespace_configs parent USING (namespace) WHERE parent.namespace IS NULL)
       OR EXISTS (SELECT 1 FROM id_mappings child LEFT JOIN namespace_configs parent USING (namespace) WHERE parent.namespace IS NULL)
       OR EXISTS (SELECT 1 FROM batch_run_logs child LEFT JOIN namespace_configs parent USING (namespace) WHERE parent.namespace IS NULL)
       OR EXISTS (SELECT 1 FROM catalog_items child LEFT JOIN namespace_configs parent USING (namespace) WHERE parent.namespace IS NULL)
       OR EXISTS (SELECT 1 FROM catalog_backlog_samples child LEFT JOIN namespace_configs parent USING (namespace) WHERE parent.namespace IS NULL)
       OR EXISTS (SELECT 1 FROM objects child LEFT JOIN namespace_configs parent USING (namespace) WHERE parent.namespace IS NULL)
    THEN
        RAISE EXCEPTION 'namespace integrity preflight failed: orphan rows must be repaired before migration 025';
    END IF;
END $$;

ALTER TABLE events ADD CONSTRAINT events_namespace_fk
    FOREIGN KEY (namespace) REFERENCES namespace_configs(namespace) ON DELETE CASCADE NOT VALID;
ALTER TABLE id_mappings ADD CONSTRAINT id_mappings_namespace_fk
    FOREIGN KEY (namespace) REFERENCES namespace_configs(namespace) ON DELETE CASCADE NOT VALID;
ALTER TABLE batch_run_logs ADD CONSTRAINT batch_run_logs_namespace_fk
    FOREIGN KEY (namespace) REFERENCES namespace_configs(namespace) ON DELETE CASCADE NOT VALID;
ALTER TABLE catalog_items ADD CONSTRAINT catalog_items_namespace_fk
    FOREIGN KEY (namespace) REFERENCES namespace_configs(namespace) ON DELETE CASCADE NOT VALID;
ALTER TABLE catalog_backlog_samples ADD CONSTRAINT catalog_backlog_samples_namespace_fk
    FOREIGN KEY (namespace) REFERENCES namespace_configs(namespace) ON DELETE CASCADE NOT VALID;
ALTER TABLE objects ADD CONSTRAINT objects_namespace_fk
    FOREIGN KEY (namespace) REFERENCES namespace_configs(namespace) ON DELETE CASCADE NOT VALID;

ALTER TABLE events VALIDATE CONSTRAINT events_namespace_fk;
ALTER TABLE id_mappings VALIDATE CONSTRAINT id_mappings_namespace_fk;
ALTER TABLE batch_run_logs VALIDATE CONSTRAINT batch_run_logs_namespace_fk;
ALTER TABLE catalog_items VALIDATE CONSTRAINT catalog_items_namespace_fk;
ALTER TABLE catalog_backlog_samples VALIDATE CONSTRAINT catalog_backlog_samples_namespace_fk;
ALTER TABLE objects VALIDATE CONSTRAINT objects_namespace_fk;

CREATE INDEX idx_catalog_items_ns_updated_id
    ON catalog_items (namespace, updated_at, id);
