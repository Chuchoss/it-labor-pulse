-- Manual assistant runs scan a finite vacancy snapshot independently of the ingest outbox.
ALTER TABLE assistant_runs
    ADD COLUMN snapshot_cutoff TIMESTAMPTZ,
    ADD COLUMN snapshot_total INTEGER NOT NULL DEFAULT 0 CHECK (snapshot_total >= 0),
    ADD COLUMN snapshot_cursor_created_at TIMESTAMPTZ,
    ADD COLUMN snapshot_cursor_vacancy_id UUID,
    ADD COLUMN preference_id UUID REFERENCES vacancy_preferences(id),
    ADD COLUMN lease_until TIMESTAMPTZ;

CREATE INDEX assistant_runs_recoverable
    ON assistant_runs (lease_until)
    WHERE state = 'running';

CREATE UNIQUE INDEX assistant_runs_one_active_per_user
    ON assistant_runs (user_id)
    WHERE state IN ('queued', 'running');

CREATE UNIQUE INDEX assistant_runs_request_idempotency
    ON assistant_runs (user_id, request_id)
    WHERE request_id IS NOT NULL;

CREATE INDEX vacancies_assistant_snapshot_scan
    ON vacancies (created_at, id)
    WHERE is_active AND deleted_at IS NULL;

COMMENT ON COLUMN assistant_runs.snapshot_cutoff IS
    'Stable upper created_at bound captured when a manual full-snapshot run is created.';
COMMENT ON COLUMN assistant_runs.snapshot_total IS
    'Eligible active vacancies from active sources at run creation.';
