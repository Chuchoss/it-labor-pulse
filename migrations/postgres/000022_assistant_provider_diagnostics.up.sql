ALTER TABLE assistant_runs
    DROP CONSTRAINT assistant_runs_state_check,
    ADD CONSTRAINT assistant_runs_state_check
        CHECK (state IN ('queued', 'running', 'paused', 'succeeded', 'failed', 'disabled')),
    ADD COLUMN ai_http_attempts INTEGER NOT NULL DEFAULT 0 CHECK (ai_http_attempts >= 0),
    ADD COLUMN ai_retries INTEGER NOT NULL DEFAULT 0 CHECK (ai_retries >= 0),
    ADD COLUMN ai_rate_limit INTEGER NOT NULL DEFAULT 0 CHECK (ai_rate_limit >= 0),
    ADD COLUMN ai_timeouts INTEGER NOT NULL DEFAULT 0 CHECK (ai_timeouts >= 0),
    ADD COLUMN ai_invalid_responses INTEGER NOT NULL DEFAULT 0 CHECK (ai_invalid_responses >= 0),
    ADD COLUMN ai_auth INTEGER NOT NULL DEFAULT 0 CHECK (ai_auth >= 0),
    ADD COLUMN ai_quota INTEGER NOT NULL DEFAULT 0 CHECK (ai_quota >= 0),
    ADD COLUMN ai_server INTEGER NOT NULL DEFAULT 0 CHECK (ai_server >= 0),
    ADD COLUMN ai_network INTEGER NOT NULL DEFAULT 0 CHECK (ai_network >= 0),
    ADD COLUMN ai_context_limit INTEGER NOT NULL DEFAULT 0 CHECK (ai_context_limit >= 0),
    ADD COLUMN ai_content_filter INTEGER NOT NULL DEFAULT 0 CHECK (ai_content_filter >= 0),
    ADD COLUMN ai_invalid_request INTEGER NOT NULL DEFAULT 0 CHECK (ai_invalid_request >= 0);

-- Before this migration one ai_calls unit represented one HTTP request.
UPDATE assistant_runs SET ai_http_attempts = ai_calls;

DROP INDEX assistant_runs_one_active_per_user;
CREATE UNIQUE INDEX assistant_runs_one_active_per_user
    ON assistant_runs (user_id)
    WHERE state IN ('queued', 'running', 'paused');

COMMENT ON COLUMN assistant_runs.ai_calls IS
    'Vacancies sent to the AI provider; retries are not included.';
COMMENT ON COLUMN assistant_runs.ai_http_attempts IS
    'All provider HTTP attempts, including retries.';
COMMENT ON COLUMN assistant_runs.ai_retries IS
    'Provider HTTP attempts after the first attempt for a vacancy.';
