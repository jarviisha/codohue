-- Restore author_subject_id on catalog_items and backfill from objects.
--
-- Lossy by nature: attribution for an object that has no catalog_items row
-- (the whole reason the objects table exists) cannot be represented here and
-- is dropped along with the table.

ALTER TABLE catalog_items ADD COLUMN author_subject_id TEXT;

UPDATE catalog_items c
SET author_subject_id = o.author_subject_id
FROM objects o
WHERE o.namespace = c.namespace
  AND o.object_id = c.object_id
  AND o.author_subject_id IS NOT NULL;

CREATE INDEX idx_catalog_items_ns_author
    ON catalog_items (namespace, author_subject_id)
    WHERE author_subject_id IS NOT NULL;

DROP TABLE objects;
