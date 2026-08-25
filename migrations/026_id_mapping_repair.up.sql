-- Composite numeric-ID uniqueness plus the durable manifest the migration-022
-- reconciliation runs from.
--
-- Migration 022 re-keyed id_mappings on (namespace, entity_type, string_id) but
-- left numeric_id globally UNIQUE. That is stricter than the data requires and
-- weaker than it needs: Qdrant collections are per (namespace, generation,
-- kind), so two namespaces may safely hold the same numeric id, while two
-- logical ids inside ONE collection must never share one. Scoping the
-- constraint to (namespace, entity_type) states exactly that.
ALTER TABLE id_mappings DROP CONSTRAINT IF EXISTS id_mappings_numeric_id_key;

ALTER TABLE id_mappings
    ADD CONSTRAINT id_mappings_numeric_id_scoped_key
    UNIQUE (namespace, entity_type, numeric_id);

-- One row per reconciliation run. The run is a state machine rather than a
-- single transaction because it spans two stores: PostgreSQL mappings and
-- Qdrant points cannot commit together, so a crash has to be resumable from
-- durable state instead of rolled back.
CREATE TABLE id_mapping_repair_runs (
    id                   BIGSERIAL   PRIMARY KEY,
    state                TEXT        NOT NULL,
    pg_snapshot_ref      TEXT,
    qdrant_snapshot_refs JSONB       NOT NULL DEFAULT '{}'::jsonb,
    -- Hash of the audited item set. Apply refuses to run against a manifest
    -- that changed after it was audited: the operator reviewed a specific set
    -- of decisions, and silently applying a different one is the failure mode
    -- this whole workflow exists to prevent.
    manifest_hash        TEXT        NOT NULL DEFAULT '',
    started_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at         TIMESTAMPTZ,
    error                TEXT,
    CONSTRAINT id_mapping_repair_runs_state_chk CHECK (
        state IN ('audited', 'snapshotting', 'applying', 'verifying', 'complete', 'failed')
    )
);

-- Only one run may be in flight: two concurrent reconciliations would each
-- believe they own the numeric-id space.
CREATE UNIQUE INDEX idx_id_mapping_repair_runs_active
    ON id_mapping_repair_runs ((TRUE))
    WHERE state NOT IN ('complete', 'failed');

-- One audited tuple per logical identity. old_numeric_ids is an array because
-- the whole point of the audit is finding identities that resolve to more than
-- one numeric id across stores.
CREATE TABLE id_mapping_repair_items (
    run_id            BIGINT      NOT NULL REFERENCES id_mapping_repair_runs(id) ON DELETE CASCADE,
    namespace         TEXT        NOT NULL,
    entity_type       TEXT        NOT NULL,
    string_id         TEXT        NOT NULL,
    old_numeric_ids   BIGINT[]    NOT NULL DEFAULT '{}',
    target_numeric_id BIGINT,
    sources           JSONB       NOT NULL DEFAULT '{}'::jsonb,
    payload_hash      TEXT,
    vector_hash       TEXT,
    state             TEXT        NOT NULL DEFAULT 'pending',
    error             TEXT,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (run_id, namespace, entity_type, string_id),
    CONSTRAINT id_mapping_repair_items_state_chk CHECK (
        state IN ('pending', 'copied', 'verified', 'cleaned', 'quarantined', 'failed')
    ),
    CONSTRAINT id_mapping_repair_items_entity_chk CHECK (
        entity_type IN ('subject', 'object')
    )
);

-- Resume reads "what is left to do in this run", and the quarantine report
-- reads "what stopped it".
CREATE INDEX idx_id_mapping_repair_items_run_state
    ON id_mapping_repair_items (run_id, state);
