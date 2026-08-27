ALTER TABLE assistant_runs
    DROP COLUMN ai_cached_tokens,
    DROP COLUMN ai_completion_tokens,
    DROP COLUMN ai_prompt_tokens,
    DROP COLUMN ai_batches;
