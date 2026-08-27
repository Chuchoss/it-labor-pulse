ALTER TABLE assistant_runs
    ADD COLUMN worker_heartbeat_at TIMESTAMPTZ,
    ADD COLUMN worker_phase TEXT NOT NULL DEFAULT 'idle'
        CHECK (worker_phase IN ('idle', 'processing', 'provider_request', 'backoff', 'stopping')),
    ADD COLUMN worker_retry_category TEXT,
    ADD COLUMN worker_retry_until TIMESTAMPTZ;

COMMENT ON COLUMN assistant_runs.worker_heartbeat_at IS
    'Worker liveness heartbeat; independent from durable batch progress in last_checked_at.';
COMMENT ON COLUMN assistant_runs.worker_phase IS
    'Safe coarse worker phase exposed to operators without provider payloads.';
COMMENT ON COLUMN assistant_runs.worker_retry_category IS
    'Stable provider retry category; never contains raw provider response data.';
COMMENT ON COLUMN assistant_runs.worker_retry_until IS
    'Scheduled end of the current provider backoff.';
