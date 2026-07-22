-- Opt-in filter: drop objects the requesting subject authored from that
-- subject's own recommendations.
--
-- Default FALSE on purpose. author_subject_id (migration 019) is ownership
-- metadata that nothing in the recommendation path reads; namespaces may be
-- sending it purely for display. Turning the filter on by default would
-- silently change recommendation output for them, so it stays an explicit
-- per-namespace choice.
--
-- Inert unless the namespace actually has authors, which today means
-- dense_source='catalog' (only catalog_items carries the column).

ALTER TABLE namespace_configs
  ADD COLUMN exclude_authored BOOLEAN NOT NULL DEFAULT FALSE;
