-- Author attribution for catalog items.
--
-- author_subject_id shares the id space of events.subject_id, but there is
-- deliberately no foreign key: subjects are not a stored resource in Codohue,
-- they only exist as ids inside events. The column records "who created this
-- object", which is ownership metadata — it is NOT a behavioural link and must
-- never be read as one. The subject↔object interaction graph lives solely in
-- the events table, and collaborative filtering depends on that relation being
-- many-to-many.
--
-- Nullable because attribution is optional: an object may have no author
-- (system-generated content), and existing rows predate the column. The index
-- is partial for the same reason — only attributed rows are worth indexing.

ALTER TABLE catalog_items ADD COLUMN author_subject_id TEXT;

CREATE INDEX idx_catalog_items_ns_author
  ON catalog_items (namespace, author_subject_id)
  WHERE author_subject_id IS NOT NULL;
