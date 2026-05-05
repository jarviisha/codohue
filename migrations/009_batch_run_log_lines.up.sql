ALTER TABLE batch_run_logs
  ADD COLUMN log_lines JSONB NOT NULL DEFAULT '[]';
