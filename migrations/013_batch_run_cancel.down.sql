DROP INDEX IF EXISTS idx_batch_run_logs_running_cancel;
ALTER TABLE batch_run_logs DROP COLUMN IF EXISTS cancel_requested;
