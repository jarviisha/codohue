CREATE TABLE namespace_configs (
    namespace         TEXT        PRIMARY KEY,
    action_weights    JSONB       NOT NULL DEFAULT '{}',
    time_decay_factor FLOAT       NOT NULL DEFAULT 0.95,
    max_results       INTEGER     NOT NULL DEFAULT 50,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE events (
    id          BIGSERIAL   PRIMARY KEY,
    namespace   TEXT        NOT NULL,
    subject_id  TEXT        NOT NULL,
    object_id   TEXT        NOT NULL,
    action      TEXT        NOT NULL,
    weight      FLOAT       NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_events_namespace_subject ON events (namespace, subject_id);
CREATE INDEX idx_events_occurred_at ON events (occurred_at);

-- Batch job dùng full recompute từ 90 ngày, không cần tracked flag.

CREATE TABLE id_mappings (
    string_id   TEXT        PRIMARY KEY,
    numeric_id  BIGSERIAL   UNIQUE NOT NULL,
    namespace   TEXT        NOT NULL,
    entity_type TEXT        NOT NULL,  -- 'subject' | 'object'
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
