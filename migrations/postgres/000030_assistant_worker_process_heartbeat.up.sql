CREATE TABLE assistant_worker_instances (
    instance_id UUID PRIMARY KEY,
    started_at TIMESTAMPTZ NOT NULL,
    version TEXT NOT NULL CHECK (length(version) BETWEEN 1 AND 100),
    mode TEXT NOT NULL CHECK (length(mode) BETWEEN 1 AND 50),
    state TEXT NOT NULL CHECK (state IN ('idle', 'processing', 'backoff', 'stopping')),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
);

CREATE INDEX assistant_worker_instances_last_seen_idx
    ON assistant_worker_instances (last_seen_at DESC);

COMMENT ON TABLE assistant_worker_instances IS
    'Process-level assistant worker liveness. Rows expire by last_seen_at and are pruned by heartbeat upserts.';
COMMENT ON COLUMN assistant_worker_instances.state IS
    'Safe coarse process state; independent from assistant run leases and provider payloads.';
