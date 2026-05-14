-- Lock batch_run_logs.trigger_source to the values the Go enum in
-- internal/core/batchrun knows about. Any future trigger source needs a
-- companion migration that widens this constraint.

ALTER TABLE batch_run_logs
  ADD CONSTRAINT batch_run_logs_trigger_source_check
  CHECK (trigger_source IN ('cron', 'manual', 'admin_reembed'));
