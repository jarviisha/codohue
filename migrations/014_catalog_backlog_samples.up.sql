-- Phase 2 — persisted catalog backlog snapshots.
--
-- cmd/embedder samples backlog counts every 30 seconds; rows here back the
-- /catalog/backlog-history endpoint so the timeline survives reload + lets
-- the SPA render trends beyond the in-memory client buffer. Retention is
-- handled by a periodic delete in cmd/cron (BUILD_PLAN §8 migration 014
-- "Sampler skip rule" + retention).

CREATE TABLE catalog_backlog_samples (
  namespace    text        NOT NULL,
  sampled_at   timestamptz NOT NULL,
  pending      integer     NOT NULL,
  in_flight    integer     NOT NULL,
  failed       integer     NOT NULL,
  dead_letter  integer     NOT NULL,
  stream_len   integer     NOT NULL,
  PRIMARY KEY (namespace, sampled_at)
);

-- Time-series queries scan a window per namespace ordered newest-first, so
-- the index leads with namespace and orders by sampled_at desc to match
-- predicate + ORDER BY without a sort step.
CREATE INDEX idx_catalog_backlog_samples_ns_time
  ON catalog_backlog_samples (namespace, sampled_at DESC);
