-- Operator pause switch. Non-NULL means the namespace is paused: the data
-- plane rejects its requests, stream ingest drops its messages, and cron and
-- the embedder skip it. Orthogonal to namespace_lifecycles, which only models
-- delete/reset; the revision trigger ignores this column on purpose.
ALTER TABLE namespace_configs ADD COLUMN paused_at TIMESTAMPTZ;
