ALTER TABLE assistant_runs
    ADD COLUMN ai_reviews INTEGER NOT NULL DEFAULT 0 CHECK (ai_reviews >= 0),
    ADD COLUMN ai_rejects INTEGER NOT NULL DEFAULT 0 CHECK (ai_rejects >= 0),
    ADD COLUMN worker_active_batches INTEGER NOT NULL DEFAULT 0 CHECK (worker_active_batches >= 0),
    ADD COLUMN worker_concurrency INTEGER NOT NULL DEFAULT 0 CHECK (worker_concurrency >= 0);
