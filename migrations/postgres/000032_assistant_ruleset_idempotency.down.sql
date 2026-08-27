ALTER TABLE assistant_runs
    DROP CONSTRAINT assistant_runs_ai_skip_reason_check,
    ADD CONSTRAINT assistant_runs_ai_skip_reason_check
        CHECK (ai_skip_reason IN (
            'server_disabled', 'user_opt_out', 'run_predates_ai', 'no_eligible',
            'budget_exhausted', 'already_analyzed', 'provider_unavailable',
            'preferences_changed', 'unknown'
        ));

DROP INDEX IF EXISTS vacancy_match_results_revision_ruleset_dedup;
CREATE UNIQUE INDEX vacancy_match_results_revision_dedup
    ON vacancy_match_results (
        user_id, preference_id, vacancy_id, vacancy_revision, method,
        COALESCE(provider, ''), COALESCE(model, ''), COALESCE(prompt_version, '')
    );

DROP INDEX IF EXISTS assistant_ai_jobs_revision_ruleset_dedup;
ALTER TABLE assistant_ai_jobs
    DROP COLUMN IF EXISTS ruleset_version;
CREATE UNIQUE INDEX assistant_ai_jobs_revision_dedup
    ON assistant_ai_jobs (user_id, preference_id, vacancy_id, vacancy_revision);
