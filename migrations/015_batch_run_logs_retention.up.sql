-- Phase 4 — retention support for batch_run_logs + catalog_backlog_samples.
--
-- Both tables grow unbounded under steady-state operation: every cron tick
-- writes one batch_run_logs row per namespace, and the embedder sampler
-- writes one catalog_backlog_samples row per namespace every 30s. cmd/cron
-- now runs a periodic retention task that deletes rows older than
-- CODOHUE_BATCH_RUN_RETENTION_DAYS / CODOHUE_BACKLOG_SAMPLES_RETENTION_DAYS.
--
-- This migration adds the supporting indexes — without them the retention
-- DELETE would scan the whole table. The existing per-namespace indexes
-- aren't usable because retention prunes globally by time.

CREATE INDEX IF NOT EXISTS idx_batch_run_logs_started
  ON batch_run_logs (started_at);

CREATE INDEX IF NOT EXISTS idx_catalog_backlog_samples_time
  ON catalog_backlog_samples (sampled_at);
