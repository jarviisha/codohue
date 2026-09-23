ALTER TABLE namespace_configs
 ADD COLUMN recommendations_revision bigint NOT NULL DEFAULT 1 CHECK (recommendations_revision > 0),
 ADD COLUMN signals_revision bigint NOT NULL DEFAULT 1 CHECK (signals_revision > 0),
 ADD COLUMN trending_revision bigint NOT NULL DEFAULT 1 CHECK (trending_revision > 0),
 ADD COLUMN embeddings_revision bigint NOT NULL DEFAULT 1 CHECK (embeddings_revision > 0);

CREATE FUNCTION advance_configuration_revisions() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 NEW.recommendations_revision := OLD.recommendations_revision + CASE WHEN ROW(NEW.alpha,NEW.gamma,NEW.max_results,NEW.seen_items_days,NEW.exclude_authored) IS DISTINCT FROM ROW(OLD.alpha,OLD.gamma,OLD.max_results,OLD.seen_items_days,OLD.exclude_authored) THEN 1 ELSE 0 END;
 NEW.signals_revision := OLD.signals_revision + CASE WHEN ROW(NEW.action_weights,NEW.time_decay_factor) IS DISTINCT FROM ROW(OLD.action_weights,OLD.time_decay_factor) THEN 1 ELSE 0 END;
 NEW.trending_revision := OLD.trending_revision + CASE WHEN ROW(NEW.trending_window,NEW.trending_ttl,NEW.lambda_trending) IS DISTINCT FROM ROW(OLD.trending_window,OLD.trending_ttl,OLD.lambda_trending) THEN 1 ELSE 0 END;
 NEW.embeddings_revision := OLD.embeddings_revision + CASE WHEN ROW(NEW.dense_source,NEW.embedding_dim,NEW.dense_distance,NEW.catalog_strategy_id,NEW.catalog_strategy_version,NEW.catalog_strategy_params,NEW.catalog_max_attempts,NEW.catalog_max_content_bytes) IS DISTINCT FROM ROW(OLD.dense_source,OLD.embedding_dim,OLD.dense_distance,OLD.catalog_strategy_id,OLD.catalog_strategy_version,OLD.catalog_strategy_params,OLD.catalog_max_attempts,OLD.catalog_max_content_bytes) THEN 1 ELSE 0 END;
 RETURN NEW;
END;
$$;
CREATE TRIGGER configuration_revisions BEFORE UPDATE ON namespace_configs
 FOR EACH ROW EXECUTE FUNCTION advance_configuration_revisions();
