ALTER TABLE assistant_runs
    ADD COLUMN ai_batches INTEGER NOT NULL DEFAULT 0 CHECK (ai_batches >= 0),
    ADD COLUMN ai_prompt_tokens BIGINT NOT NULL DEFAULT 0 CHECK (ai_prompt_tokens >= 0),
    ADD COLUMN ai_completion_tokens BIGINT NOT NULL DEFAULT 0 CHECK (ai_completion_tokens >= 0),
    ADD COLUMN ai_cached_tokens BIGINT NOT NULL DEFAULT 0 CHECK (ai_cached_tokens >= 0);

-- Historical calls were singleton requests. Token usage was not collected.
UPDATE assistant_runs
SET ai_batches = ai_http_attempts
WHERE ai_batches = 0 AND ai_http_attempts > 0;

COMMENT ON COLUMN assistant_runs.ai_batches IS
    'Logical provider batches, including recursively split and singleton fallback batches.';
COMMENT ON COLUMN assistant_runs.ai_prompt_tokens IS
    'Aggregate provider-reported prompt tokens; zero means unknown for historical runs.';
COMMENT ON COLUMN assistant_runs.ai_completion_tokens IS
    'Aggregate provider-reported completion tokens; zero means unknown for historical runs.';
COMMENT ON COLUMN assistant_runs.ai_cached_tokens IS
    'Aggregate provider-reported prompt cache hit tokens when available.';
