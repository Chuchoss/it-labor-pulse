DROP INDEX IF EXISTS vacancies_assistant_snapshot_scan;
DROP INDEX IF EXISTS assistant_runs_request_idempotency;
DROP INDEX IF EXISTS assistant_runs_one_active_per_user;
DROP INDEX IF EXISTS assistant_runs_recoverable;

ALTER TABLE assistant_runs
    DROP COLUMN IF EXISTS lease_until,
    DROP COLUMN IF EXISTS preference_id,
    DROP COLUMN IF EXISTS snapshot_cursor_vacancy_id,
    DROP COLUMN IF EXISTS snapshot_cursor_created_at,
    DROP COLUMN IF EXISTS snapshot_total,
    DROP COLUMN IF EXISTS snapshot_cutoff;
