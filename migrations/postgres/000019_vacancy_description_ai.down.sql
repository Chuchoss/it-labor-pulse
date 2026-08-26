ALTER TABLE assistant_runs
    DROP COLUMN ai_skipped,
    DROP COLUMN ai_failures,
    DROP COLUMN ai_matches;

DROP INDEX vacancy_match_results_revision_dedup;
ALTER TABLE vacancy_match_results DROP COLUMN vacancy_revision;
ALTER TABLE vacancy_match_results
    ADD CONSTRAINT vacancy_match_results_user_id_preference_id_vacancy_id_meth_key
    UNIQUE (user_id, preference_id, vacancy_id, method, provider, model, prompt_version);
CREATE UNIQUE INDEX vacancy_match_results_dedup
    ON vacancy_match_results (
        user_id, preference_id, vacancy_id, method,
        COALESCE(provider, ''), COALESCE(model, ''), COALESCE(prompt_version, '')
    );

DROP INDEX assistant_ai_jobs_revision_dedup;
ALTER TABLE assistant_ai_jobs DROP COLUMN vacancy_revision;
ALTER TABLE assistant_ai_jobs
    ADD CONSTRAINT assistant_ai_jobs_user_id_preference_id_vacancy_id_key
    UNIQUE (user_id, preference_id, vacancy_id);

DROP INDEX assistant_work_items_revision_dedup;
ALTER TABLE assistant_work_items DROP COLUMN vacancy_revision;
ALTER TABLE assistant_work_items
    ADD CONSTRAINT assistant_work_items_source_external_id_key
    UNIQUE (source, external_id);

ALTER TABLE vacancies
    DROP COLUMN analysis_revision,
    DROP COLUMN description_truncated;
