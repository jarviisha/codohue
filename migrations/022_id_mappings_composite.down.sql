-- Restore the global string_id primary key.
--
-- The preflight runs BEFORE any constraint is touched. Once the composite key
-- has minted rows sharing a string_id across namespaces or entity types, the
-- global key cannot be restored: PRIMARY KEY (string_id) would fail on the
-- duplicates, and it would fail *after* the old key was already dropped,
-- leaving id_mappings with no primary key at all. Refusing up front keeps the
-- table in a known-good state.
--
-- There is no lossless automatic merge — two namespaces that both use "user-1"
-- legitimately mean two different subjects, and collapsing them would silently
-- merge their behaviour. Resolve them deliberately (see
-- deploy/idmap-repair-runbook.md) before rolling back.
DO $$
DECLARE
    duplicate_count BIGINT;
    sample_id TEXT;
BEGIN
    SELECT COUNT(*), MIN(string_id) INTO duplicate_count, sample_id FROM (
        SELECT string_id FROM id_mappings GROUP BY string_id HAVING COUNT(*) > 1
    ) AS duplicates;

    IF duplicate_count > 0 THEN
        RAISE EXCEPTION
            'refusing to restore the global string_id primary key: % string id(s) are used in more than one (namespace, entity_type) scope, e.g. %. This migration is forward-only once duplicates exist; see deploy/idmap-repair-runbook.md.',
            duplicate_count, sample_id;
    END IF;
END $$;

ALTER TABLE id_mappings DROP CONSTRAINT id_mappings_pkey;
ALTER TABLE id_mappings ADD PRIMARY KEY (string_id);
