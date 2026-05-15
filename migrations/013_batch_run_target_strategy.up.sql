-- Replace the error_message-overload hack used by catalog re-embed runs with
-- proper columns. Pre-feature-004 rows have no encoded target, so the new
-- columns stay NULL for them. The encoding helper in catalog_ops_repository
-- (parseReembedTarget) goes away once writers populate these columns.

ALTER TABLE batch_run_logs
  ADD COLUMN target_strategy_id      text,
  ADD COLUMN target_strategy_version text;
