CREATE TABLE batch_run_logs (
    id                  BIGSERIAL PRIMARY KEY,
    namespace           TEXT NOT NULL,
    started_at          TIMESTAMPTZ NOT NULL,
    completed_at        TIMESTAMPTZ,
    duration_ms         INTEGER,
    subjects_processed  INTEGER NOT NULL DEFAULT 0,
    success             BOOLEAN NOT NULL DEFAULT FALSE,
    error_message       TEXT
);

CREATE INDEX idx_batch_run_logs_ns_started
    ON batch_run_logs (namespace, started_at DESC);
