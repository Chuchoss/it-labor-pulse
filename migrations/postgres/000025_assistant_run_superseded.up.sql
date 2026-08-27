DROP INDEX assistant_runs_one_active_per_user;

ALTER TABLE assistant_runs
    DROP CONSTRAINT assistant_runs_state_check,
    ADD CONSTRAINT assistant_runs_state_check
        CHECK (state IN (
            'queued', 'running', 'paused', 'succeeded', 'failed', 'disabled',
            'superseded'
        )),
    ADD COLUMN superseded_at TIMESTAMPTZ,
    ADD COLUMN superseded_by_preference_id UUID REFERENCES vacancy_preferences(id);

ALTER TABLE assistant_runs
    DROP CONSTRAINT assistant_runs_ai_skip_reason_check,
    ADD CONSTRAINT assistant_runs_ai_skip_reason_check
        CHECK (ai_skip_reason IN (
            'server_disabled', 'user_opt_out', 'run_predates_ai', 'no_eligible',
            'budget_exhausted', 'already_analyzed', 'provider_unavailable',
            'preferences_changed', 'unknown'
        ));

CREATE UNIQUE INDEX assistant_runs_one_active_per_user
    ON assistant_runs (user_id)
    WHERE state IN ('queued', 'running', 'paused');

COMMENT ON COLUMN assistant_runs.superseded_at IS
    'Terminal time when a manual snapshot was stopped because a newer preference version exists.';
COMMENT ON COLUMN assistant_runs.superseded_by_preference_id IS
    'Newer immutable preference version that made this manual snapshot obsolete.';
