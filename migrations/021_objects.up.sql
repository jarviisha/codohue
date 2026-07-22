-- Per-object metadata that must work in every dense_source mode.
--
-- author_subject_id lived on catalog_items (migration 019), which only exists
-- for namespaces whose content goes through catalog auto-embedding. Under
-- item2vec / svd / byoe an object has no row anywhere — it is just an id
-- inside events — so attribution had no home and exclude_authored silently
-- did nothing there.
--
-- This table is that home. It is deliberately NOT tied to embedding: no
-- content, no state machine, no strategy columns. catalog_items stays the
-- store for embedding input; `objects` is the store for facts about the
-- object itself.
--
-- The column is MOVED, not copied. Two stores for one fact drift apart —
-- the same failure mode migration 016/017 removed when dense_strategy and
-- catalog_enabled could disagree about catalog mode.
--
-- No foreign key to namespace_configs (matching every other table here) and
-- none to subjects, which are not a stored resource at all.

CREATE TABLE objects (
    namespace         TEXT        NOT NULL,
    object_id         TEXT        NOT NULL,
    author_subject_id TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (namespace, object_id)
);

-- Partial, like idx_catalog_items_ns_author was: only attributed rows are
-- worth indexing, and the only query is "objects authored by this subject".
CREATE INDEX idx_objects_ns_author
    ON objects (namespace, author_subject_id)
    WHERE author_subject_id IS NOT NULL;

INSERT INTO objects (namespace, object_id, author_subject_id, created_at, updated_at)
SELECT namespace, object_id, author_subject_id, created_at, updated_at
FROM catalog_items
WHERE author_subject_id IS NOT NULL
ON CONFLICT (namespace, object_id) DO NOTHING;

DROP INDEX IF EXISTS idx_catalog_items_ns_author;

ALTER TABLE catalog_items DROP COLUMN author_subject_id;
