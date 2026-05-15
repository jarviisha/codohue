-- "subjects_processed" was misleading once catalog re-embed runs started
-- writing to the same column with a different meaning (items, not subjects).
-- Rename to a kind-neutral term so the column reads sensibly across both
-- CF runs (count of subjects re-vectored) and re-embed runs (count of
-- catalog items reset for embedding).

ALTER TABLE batch_run_logs
  RENAME COLUMN subjects_processed TO entities_processed;
