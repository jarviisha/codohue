DROP TRIGGER configuration_revisions ON namespace_configs;
DROP FUNCTION advance_configuration_revisions();
ALTER TABLE namespace_configs DROP COLUMN recommendations_revision, DROP COLUMN signals_revision, DROP COLUMN trending_revision, DROP COLUMN embeddings_revision;
