ALTER TABLE assistant_runs
    ADD COLUMN ruleset_version TEXT NOT NULL DEFAULT 'legacy';

ALTER TABLE vacancy_match_results
    ADD COLUMN run_id UUID REFERENCES assistant_runs(id),
    ADD COLUMN ruleset_version TEXT NOT NULL DEFAULT 'legacy';

CREATE INDEX vacancy_match_results_current_run
    ON vacancy_match_results (user_id, run_id, ruleset_version, created_at DESC)
    WHERE decision = 'match' AND method = 'ai';

COMMENT ON COLUMN assistant_runs.ruleset_version IS
    'Immutable matcher and AI policy version used by this run.';
COMMENT ON COLUMN vacancy_match_results.run_id IS
    'Manual snapshot run that produced the result; NULL denotes prospective outbox processing.';
COMMENT ON COLUMN vacancy_match_results.ruleset_version IS
    'Hard-gate policy version used to calculate the final result.';
