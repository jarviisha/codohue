-- Restore the BIGSERIAL default ceiling. The uint32 limit still applies in
-- internal/compute; without the cap, crossing it drops dimensions silently
-- again instead of failing the mint.
DO $$
DECLARE
    seq_name text := pg_get_serial_sequence('id_mappings', 'numeric_id');
BEGIN
    IF seq_name IS NULL THEN
        RETURN;
    END IF;

    EXECUTE format('ALTER SEQUENCE %s MAXVALUE 9223372036854775807', seq_name);
END
$$;
