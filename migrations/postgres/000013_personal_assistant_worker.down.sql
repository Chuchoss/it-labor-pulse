DROP TABLE IF EXISTS assistant_preference_requests;
DROP INDEX IF EXISTS vacancy_match_results_dedup;
DROP INDEX IF EXISTS telegram_deliveries_retryable;
DROP INDEX IF EXISTS vacancy_match_results_preference_created;
ALTER TABLE vacancy_preferences
    DROP CONSTRAINT IF EXISTS vacancy_preferences_note_length,
    DROP CONSTRAINT IF EXISTS vacancy_preferences_hard_criteria_object,
    DROP CONSTRAINT IF EXISTS vacancy_preferences_soft_criteria_object,
    DROP CONSTRAINT IF EXISTS vacancy_preferences_weights_object;
DROP TABLE IF EXISTS assistant_ai_jobs;
DROP TABLE IF EXISTS assistant_cursors;
DROP TABLE IF EXISTS assistant_work_items;
