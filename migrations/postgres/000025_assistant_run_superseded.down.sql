DROP INDEX assistant_runs_one_active_per_user;

ALTER TABLE assistant_runs
    DROP COLUMN superseded_by_preference_id,
    DROP COLUMN superseded_at,
    DROP CONSTRAINT assistant_runs_state_check,
    ADD CONSTRAINT assistant_runs_state_check
        CHECK (state IN (
            'queued', 'running', 'paused', 'succeeded', 'failed', 'disabled'
        )),
    DROP CONSTRAINT assistant_runs_ai_skip_reason_check,
    ADD CONSTRAINT assistant_runs_ai_skip_reason_check
        CHECK (ai_skip_reason IN (
            'server_disabled', 'user_opt_out', 'run_predates_ai', 'no_eligible',
            'budget_exhausted', 'already_analyzed', 'provider_unavailable',
            'unknown'
        ));

CREATE UNIQUE INDEX assistant_runs_one_active_per_user
    ON assistant_runs (user_id)
    WHERE state IN ('queued', 'running', 'paused');
