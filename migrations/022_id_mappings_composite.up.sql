-- Re-key id_mappings on (namespace, entity_type, string_id).
--
-- The original PRIMARY KEY (string_id) was global: the same string used in two
-- namespaces (or as both a subject and an object) shared one row stamped with
-- the first writer's namespace/entity_type. Namespace wipes then deleted rows
-- other namespaces depended on, and object-scoped lookups missed rows first
-- created as subjects.
--
-- Existing rows keep their original namespace/entity_type; a namespace that was
-- borrowing another namespace's row mints a fresh numeric_id on its next
-- GetOrCreate. After applying this migration, run a full recompute for every
-- namespace so Qdrant points are rebuilt against the new numeric ids.
ALTER TABLE id_mappings DROP CONSTRAINT id_mappings_pkey;
ALTER TABLE id_mappings ADD PRIMARY KEY (namespace, entity_type, string_id);
