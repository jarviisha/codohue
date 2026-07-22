DROP INDEX IF EXISTS idx_catalog_items_ns_author;

ALTER TABLE catalog_items DROP COLUMN IF EXISTS author_subject_id;
