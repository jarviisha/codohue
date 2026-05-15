ALTER TABLE batch_run_logs
  RENAME COLUMN entities_processed TO subjects_processed;

ALTER TABLE batch_run_logs
  DROP COLUMN IF EXISTS target_strategy_id,
  DROP COLUMN IF EXISTS target_strategy_version;

ALTER TABLE batch_run_logs
  DROP CONSTRAINT IF EXISTS batch_run_logs_trigger_source_check;
