-- Supporting index for the admin subject browser
-- (GET /api/admin/v1/namespaces/{ns}/subjects).
--
-- That endpoint groups the events table by subject_id to produce
-- (subject_id, interaction_count, last_seen) rows. The existing
-- idx_events_namespace_subject covers the grouping, but MAX(occurred_at)
-- forces a heap fetch for every event in the namespace. Appending
-- occurred_at makes the aggregate an index-only scan, and keeps the
-- subject_id prefix search a plain range scan on the same index.

CREATE INDEX IF NOT EXISTS idx_events_ns_subject_occurred
  ON events (namespace, subject_id, occurred_at DESC);
