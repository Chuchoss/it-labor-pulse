ALTER TABLE assistant_runs
    DROP COLUMN worker_concurrency,
    DROP COLUMN worker_active_batches,
    DROP COLUMN ai_rejects,
    DROP COLUMN ai_reviews;
