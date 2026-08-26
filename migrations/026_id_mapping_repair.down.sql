-- Reverse 026, refusing before any constraint mutation when the data can no
-- longer satisfy the global uniqueness this restores.
--
-- The preflight runs FIRST and deliberately raises: restoring a global UNIQUE
-- on numeric_id after two namespaces have legitimately minted the same id
-- would fail halfway through, leaving the table with neither constraint. An
-- explicit refusal keeps the schema in a known state.
DO $$
DECLARE
    duplicate_count BIGINT;
BEGIN
    SELECT COUNT(*) INTO duplicate_count FROM (
        SELECT numeric_id FROM id_mappings GROUP BY numeric_id HAVING COUNT(*) > 1
    ) AS duplicates;

    IF duplicate_count > 0 THEN
        RAISE EXCEPTION
            'refusing to restore global numeric_id uniqueness: % numeric id(s) are shared across namespace/entity scopes. Resolve them with `cmd/admin idmap-repair audit` before rolling back. The schema is UNCHANGED, but golang-migrate has now marked this version dirty: run `migrate -path migrations -database "$DATABASE_URL" force 025` before any further migration.',
            duplicate_count;
    END IF;
END $$;

DROP TABLE IF EXISTS id_mapping_repair_items;
DROP TABLE IF EXISTS id_mapping_repair_runs;

ALTER TABLE id_mappings DROP CONSTRAINT IF EXISTS id_mappings_numeric_id_scoped_key;

ALTER TABLE id_mappings
    ADD CONSTRAINT id_mappings_numeric_id_key UNIQUE (numeric_id);
