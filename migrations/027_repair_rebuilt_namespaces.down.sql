-- Drop the rebuild record. Runs in flight lose their coverage evidence, so
-- verification falls back to trusting the run phase — the behaviour before 027.
ALTER TABLE id_mapping_repair_runs
    DROP COLUMN IF EXISTS rebuilt_namespaces;
