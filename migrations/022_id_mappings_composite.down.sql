-- Restore the global string_id primary key.
--
-- This fails if the composite key has since minted rows sharing a string_id
-- across namespaces or entity types; those duplicates must be resolved
-- manually before rolling back (there is no lossless automatic merge).
ALTER TABLE id_mappings DROP CONSTRAINT id_mappings_pkey;
ALTER TABLE id_mappings ADD PRIMARY KEY (string_id);
