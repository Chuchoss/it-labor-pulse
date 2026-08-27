-- AI/deterministic idempotency is per hard-gate ruleset so a policy bump
-- can persist new outcomes without colliding with v2/v3 rows.
ALTER TABLE assistant_ai_jobs
    ADD COLUMN ruleset_version TEXT NOT NULL DEFAULT 'legacy';

DROP INDEX IF EXISTS assistant_ai_jobs_revision_dedup;
CREATE UNIQUE INDEX assistant_ai_jobs_revision_ruleset_dedup
    ON assistant_ai_jobs (user_id, preference_id, vacancy_id, vacancy_revision, ruleset_version);

DROP INDEX IF EXISTS vacancy_match_results_revision_dedup;
CREATE UNIQUE INDEX vacancy_match_results_revision_ruleset_dedup
    ON vacancy_match_results (
        user_id, preference_id, vacancy_id, vacancy_revision, method, ruleset_version,
        COALESCE(provider, ''), COALESCE(model, ''), COALESCE(prompt_version, '')
    );

ALTER TABLE assistant_runs
    DROP CONSTRAINT assistant_runs_ai_skip_reason_check,
    ADD CONSTRAINT assistant_runs_ai_skip_reason_check
        CHECK (ai_skip_reason IN (
            'server_disabled', 'user_opt_out', 'run_predates_ai', 'no_eligible',
            'budget_exhausted', 'already_analyzed', 'provider_unavailable',
            'preferences_changed', 'hard_reject', 'unknown'
        ));

COMMENT ON COLUMN assistant_ai_jobs.ruleset_version IS
    'Hard-gate policy version of this AI job; complete jobs of another ruleset do not skip reprocessing.';
COMMENT ON INDEX vacancy_match_results_revision_ruleset_dedup IS
    'One stored outcome per user, preference, vacancy revision, method, provider identity and ruleset.';
