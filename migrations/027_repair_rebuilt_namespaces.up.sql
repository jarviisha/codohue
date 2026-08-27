-- Record which namespaces had their sparse vectors rebuilt during a repair run.
--
-- Verification is the gate before the fleet unlocks, and the plan asks it to
-- confirm the sparse rebuild. Without a durable record it could only infer that
-- from the run having reached the verifying phase, which proves the code path
-- was taken, not that a given namespace was covered. This also answers "was
-- this namespace rebuilt?" during a post-incident review.
ALTER TABLE id_mapping_repair_runs
    ADD COLUMN rebuilt_namespaces JSONB NOT NULL DEFAULT '[]'::jsonb;
