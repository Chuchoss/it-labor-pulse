-- Description-aware vacancy revisions and per-revision assistant outcomes.
ALTER TABLE vacancies
    ADD COLUMN description_truncated BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN analysis_revision INTEGER NOT NULL DEFAULT 1
        CHECK (analysis_revision > 0);

ALTER TABLE assistant_work_items
    ADD COLUMN vacancy_revision INTEGER NOT NULL DEFAULT 1
        CHECK (vacancy_revision > 0);

ALTER TABLE assistant_work_items
    DROP CONSTRAINT assistant_work_items_source_external_id_key;

CREATE UNIQUE INDEX assistant_work_items_revision_dedup
    ON assistant_work_items (source, external_id, vacancy_revision);

ALTER TABLE assistant_ai_jobs
    ADD COLUMN vacancy_revision INTEGER NOT NULL DEFAULT 1
        CHECK (vacancy_revision > 0);

ALTER TABLE assistant_ai_jobs
    DROP CONSTRAINT assistant_ai_jobs_user_id_preference_id_vacancy_id_key;

CREATE UNIQUE INDEX assistant_ai_jobs_revision_dedup
    ON assistant_ai_jobs (user_id, preference_id, vacancy_id, vacancy_revision);

ALTER TABLE vacancy_match_results
    ADD COLUMN vacancy_revision INTEGER NOT NULL DEFAULT 1
        CHECK (vacancy_revision > 0);

DROP INDEX vacancy_match_results_dedup;
ALTER TABLE vacancy_match_results
    DROP CONSTRAINT IF EXISTS vacancy_match_results_user_id_preference_id_vacancy_id_meth_key;

CREATE UNIQUE INDEX vacancy_match_results_revision_dedup
    ON vacancy_match_results (
        user_id, preference_id, vacancy_id, vacancy_revision, method,
        COALESCE(provider, ''), COALESCE(model, ''), COALESCE(prompt_version, '')
    );

ALTER TABLE assistant_runs
    ADD COLUMN ai_matches INTEGER NOT NULL DEFAULT 0 CHECK (ai_matches >= 0),
    ADD COLUMN ai_failures INTEGER NOT NULL DEFAULT 0 CHECK (ai_failures >= 0),
    ADD COLUMN ai_skipped INTEGER NOT NULL DEFAULT 0 CHECK (ai_skipped >= 0);

COMMENT ON COLUMN vacancies.description_truncated IS
    'True when the sanitized plain-text description exceeded the ingest limit.';
COMMENT ON COLUMN vacancies.analysis_revision IS
    'Monotonic revision incremented when normalized analysis-relevant content changes.';
