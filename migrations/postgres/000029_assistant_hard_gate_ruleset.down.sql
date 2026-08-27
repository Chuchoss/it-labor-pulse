DROP INDEX IF EXISTS vacancy_match_results_current_run;

ALTER TABLE vacancy_match_results
    DROP COLUMN IF EXISTS ruleset_version,
    DROP COLUMN IF EXISTS run_id;

ALTER TABLE assistant_runs
    DROP COLUMN IF EXISTS ruleset_version;
