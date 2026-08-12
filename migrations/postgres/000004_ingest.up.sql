-- Phase 1: ingest runs, checkpoints, errors

CREATE TABLE ingest_runs (
    id            TEXT PRIMARY KEY,
    source        TEXT NOT NULL REFERENCES sources (code),
    mode          TEXT NOT NULL,
    status        TEXT NOT NULL,
    params        JSONB NOT NULL DEFAULT '{}'::jsonb,
    requested_by  TEXT NULL,
    started_at    TIMESTAMPTZ NULL,
    finished_at   TIMESTAMPTZ NULL,
    stats         JSONB NULL,
    error_message TEXT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ingest_runs_source_created
    ON ingest_runs (source, created_at DESC);

CREATE INDEX idx_ingest_runs_active_status
    ON ingest_runs (status)
    WHERE status IN ('queued', 'running');

CREATE TABLE ingest_checkpoints (
    source     TEXT NOT NULL REFERENCES sources (code),
    scope_hash CHAR(64) NOT NULL,
    cursor     TEXT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (source, scope_hash)
);

CREATE TABLE ingest_run_errors (
    id          BIGSERIAL PRIMARY KEY,
    run_id      TEXT NOT NULL REFERENCES ingest_runs (id),
    external_id TEXT NULL,
    stage       TEXT NULL,
    message     TEXT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_ingest_run_errors_run_id ON ingest_run_errors (run_id);
