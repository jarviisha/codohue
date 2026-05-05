ALTER TABLE batch_run_logs
  ADD COLUMN trigger_source TEXT NOT NULL DEFAULT 'cron';
