-- Phase 1 — operator cancel support for in-flight batch runs.
--
-- Cron polls cancel_requested between phases. When the flag is set, the run
-- stops cleanly, sets success=false, error_message='operator_cancelled', and
-- completed_at=now(). Cancel mid-phase is intentionally not supported
-- (BUILD_PLAN §9.2) — phase boundaries are the only safe stop points.

ALTER TABLE batch_run_logs
  ADD COLUMN cancel_requested boolean NOT NULL DEFAULT false;

-- Partial index: cron looks up "in-flight runs that operators have asked to
-- cancel" — narrowing to NULL completed_at + true keeps the index tiny.
CREATE INDEX idx_batch_run_logs_running_cancel
  ON batch_run_logs (cancel_requested)
  WHERE completed_at IS NULL;
