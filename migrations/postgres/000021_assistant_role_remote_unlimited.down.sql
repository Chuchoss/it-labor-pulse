ALTER TABLE assistant_automation_settings
    DROP CONSTRAINT IF EXISTS assistant_automation_settings_max_ai_calls_per_hour_check;

UPDATE assistant_automation_settings
SET max_ai_calls_per_hour = 20
WHERE max_ai_calls_per_hour = 0 OR max_ai_calls_per_hour > 100;

ALTER TABLE assistant_automation_settings
    ALTER COLUMN max_ai_calls_per_hour SET DEFAULT 20;

ALTER TABLE assistant_automation_settings
    ADD CONSTRAINT assistant_automation_settings_max_ai_calls_per_hour_check
    CHECK (max_ai_calls_per_hour BETWEEN 1 AND 100);

ALTER TABLE vacancies
    DROP COLUMN is_remote;
