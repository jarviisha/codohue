-- Phase 2: namespace-scoped API key authentication
-- Phase 3: hybrid dense vector config fields
ALTER TABLE namespace_configs
    ADD COLUMN api_key_hash    TEXT,
    ADD COLUMN alpha           FLOAT8  NOT NULL DEFAULT 0.7,
    ADD COLUMN dense_strategy  TEXT    NOT NULL DEFAULT 'item2vec',
    ADD COLUMN embedding_dim   INTEGER NOT NULL DEFAULT 64,
    ADD COLUMN dense_distance  TEXT    NOT NULL DEFAULT 'cosine',
    ADD COLUMN trending_window INTEGER NOT NULL DEFAULT 24,
    ADD COLUMN trending_ttl    INTEGER NOT NULL DEFAULT 600,
    ADD COLUMN lambda_trending FLOAT8  NOT NULL DEFAULT 0.1;
