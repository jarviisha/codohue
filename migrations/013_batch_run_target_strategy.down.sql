ALTER TABLE batch_run_logs
  DROP COLUMN IF EXISTS target_strategy_id,
  DROP COLUMN IF EXISTS target_strategy_version;
