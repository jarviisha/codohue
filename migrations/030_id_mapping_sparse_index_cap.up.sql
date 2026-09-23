-- Cap id_mappings.numeric_id at the uint32 sparse index space.
--
-- Qdrant sparse vectors index dimensions with uint32, but numeric_id is a
-- BIGSERIAL shared by every namespace and both entity types. Past 2^32 a
-- dimension cannot be represented: internal/compute drops it from every
-- vector with a Warn, so the entity silently stops participating in sparse
-- CF while the batch run still reports success.
--
-- Capping the sequence turns that silent ceiling into a loud one: the first
-- over-limit mint fails at insert time ("nextval: reached maximum value of
-- sequence"), while the id_mapping_repair tooling (migration 026,
-- deploy/idmap-repair-runbook.md) can still compact the space. Raising the
-- ceiling for real needs a separate sparse index space, not a bigger cap.
--
-- Conditional: a deployment that already crossed 2^32 is past the point this
-- guards, and ALTER SEQUENCE would reject a MAXVALUE below the current value.
-- Leave it uncapped there and let codohue_sparse_dimensions_skipped_total
-- report the damage rather than failing the migration.
DO $$
DECLARE
    seq_name text := pg_get_serial_sequence('id_mappings', 'numeric_id');
    current_value bigint;
BEGIN
    IF seq_name IS NULL THEN
        RAISE NOTICE 'id_mappings.numeric_id has no owned sequence; skipping cap';
        RETURN;
    END IF;

    EXECUTE format('SELECT last_value FROM %s', seq_name) INTO current_value;

    IF current_value >= 4294967295 THEN
        RAISE WARNING 'id_mappings.numeric_id is already at % (>= 2^32); leaving the sequence uncapped, see deploy/idmap-repair-runbook.md', current_value;
        RETURN;
    END IF;

    EXECUTE format('ALTER SEQUENCE %s MAXVALUE 4294967295', seq_name);
END
$$;
