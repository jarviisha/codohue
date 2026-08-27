CREATE TABLE namespace_lifecycles (
    namespace              TEXT        PRIMARY KEY,
    generation             BIGINT      NOT NULL CHECK (generation >= 1),
    state                  TEXT        NOT NULL CHECK (state IN ('active', 'deleting', 'deleted')),
    activated_at           TIMESTAMPTZ NOT NULL,
    legacy_messages_allowed BOOLEAN    NOT NULL DEFAULT FALSE,
    last_error             TEXT,
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (namespace, generation)
);

CREATE TABLE system_lifecycle (
    singleton                    BOOLEAN     PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    state                        TEXT        NOT NULL CHECK (state IN ('active', 'resetting')),
    legacy_envelopes_disabled_at TIMESTAMPTZ,
    legacy_adoption_evidence     TEXT,
    last_error                   TEXT,
    updated_at                   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK ((legacy_envelopes_disabled_at IS NULL) = (legacy_adoption_evidence IS NULL))
);

INSERT INTO system_lifecycle (singleton, state) VALUES (TRUE, 'active');

INSERT INTO namespace_lifecycles
    (namespace, generation, state, activated_at, legacy_messages_allowed, updated_at)
SELECT namespace, 1, 'active', created_at, TRUE, updated_at
FROM namespace_configs;

ALTER TABLE namespace_configs
    ADD COLUMN generation BIGINT NOT NULL DEFAULT 1;

ALTER TABLE namespace_configs
    ADD CONSTRAINT namespace_configs_lifecycle_fk
    FOREIGN KEY (namespace, generation)
    REFERENCES namespace_lifecycles (namespace, generation);
