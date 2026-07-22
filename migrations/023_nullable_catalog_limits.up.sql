-- Make the per-namespace catalog limits nullable so NULL can mean "use the
-- global default" (CODOHUE_EMBED_MAX_ATTEMPTS / CODOHUE_CATALOG_MAX_CONTENT_BYTES).
--
-- With NOT NULL DEFAULT the env vars were dead knobs: every row always
-- carried a value, so the documented "global default, per-namespace
-- override" contract could never reach the env fallback. Rows still holding
-- the old schema defaults are folded to NULL — an operator who explicitly
-- chose the default gets identical behavior through the fallback chain.
ALTER TABLE namespace_configs
    ALTER COLUMN catalog_max_attempts DROP NOT NULL,
    ALTER COLUMN catalog_max_attempts DROP DEFAULT,
    ALTER COLUMN catalog_max_content_bytes DROP NOT NULL,
    ALTER COLUMN catalog_max_content_bytes DROP DEFAULT;

UPDATE namespace_configs SET catalog_max_attempts = NULL WHERE catalog_max_attempts = 5;
UPDATE namespace_configs SET catalog_max_content_bytes = NULL WHERE catalog_max_content_bytes = 32768;
