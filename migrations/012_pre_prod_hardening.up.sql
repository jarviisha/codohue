-- Pre-production hardening of batch_run_logs:
--   1. Lock trigger_source to the values internal/core/batchrun knows about.
--   2. Replace the error_message-overload hack with dedicated target_strategy
--      columns for catalog re-embed runs.
--   3. Rename subjects_processed → entities_processed; the column carries CF
--      subject counts during cron runs and catalog item counts during
--      re-embed runs, so the kind-neutral name reads better in both cases.

ALTER TABLE batch_run_logs
  ADD CONSTRAINT batch_run_logs_trigger_source_check
  CHECK (trigger_source IN ('cron', 'manual', 'admin_reembed'));

ALTER TABLE batch_run_logs
  ADD COLUMN target_strategy_id      text,
  ADD COLUMN target_strategy_version text;

ALTER TABLE batch_run_logs
  RENAME COLUMN subjects_processed TO entities_processed;
