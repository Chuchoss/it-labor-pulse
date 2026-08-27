ALTER TABLE assistant_runs
    DROP COLUMN worker_retry_until,
    DROP COLUMN worker_retry_category,
    DROP COLUMN worker_phase,
    DROP COLUMN worker_heartbeat_at;
