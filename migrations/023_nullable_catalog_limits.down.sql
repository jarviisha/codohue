-- Restore the NOT NULL columns, materialising the schema defaults into any
-- row that was relying on the global-env fallback.
UPDATE namespace_configs SET catalog_max_attempts = 5 WHERE catalog_max_attempts IS NULL;
UPDATE namespace_configs SET catalog_max_content_bytes = 32768 WHERE catalog_max_content_bytes IS NULL;

ALTER TABLE namespace_configs
    ALTER COLUMN catalog_max_attempts SET DEFAULT 5,
    ALTER COLUMN catalog_max_attempts SET NOT NULL,
    ALTER COLUMN catalog_max_content_bytes SET DEFAULT 32768,
    ALTER COLUMN catalog_max_content_bytes SET NOT NULL;
