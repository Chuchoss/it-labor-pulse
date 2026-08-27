DROP INDEX assistant_runs_one_active_per_user;
CREATE UNIQUE INDEX assistant_runs_one_active_per_user
    ON assistant_runs (user_id)
    WHERE state IN ('queued', 'running');

ALTER TABLE assistant_runs
    DROP COLUMN ai_invalid_request,
    DROP COLUMN ai_content_filter,
    DROP COLUMN ai_context_limit,
    DROP COLUMN ai_network,
    DROP COLUMN ai_server,
    DROP COLUMN ai_quota,
    DROP COLUMN ai_auth,
    DROP COLUMN ai_invalid_responses,
    DROP COLUMN ai_timeouts,
    DROP COLUMN ai_rate_limit,
    DROP COLUMN ai_retries,
    DROP COLUMN ai_http_attempts,
    DROP CONSTRAINT assistant_runs_state_check,
    ADD CONSTRAINT assistant_runs_state_check
        CHECK (state IN ('queued', 'running', 'succeeded', 'failed', 'disabled'));
