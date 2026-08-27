-- Canonical tri-state remote fact from retained official HH detail payloads.
ALTER TABLE vacancies
    ADD COLUMN is_remote BOOLEAN;

UPDATE vacancies
SET is_remote = CASE
    WHEN raw_payload -> 'schedule' ->> 'id' = 'remote'
      OR EXISTS (
          SELECT 1
          FROM jsonb_array_elements(COALESCE(raw_payload -> 'work_format', '[]'::jsonb)) AS wf
          WHERE upper(wf ->> 'id') = 'REMOTE'
      )
        THEN true
    WHEN NULLIF(raw_payload -> 'schedule' ->> 'id', '') IS NOT NULL
      OR jsonb_array_length(COALESCE(raw_payload -> 'work_format', '[]'::jsonb)) > 0
        THEN false
    ELSE NULL
END
WHERE source = 'hh' AND raw_payload IS NOT NULL;

-- Retain the legacy column for rolling compatibility; zero explicitly means
-- unlimited. New application code neither reads nor enforces this value.
ALTER TABLE assistant_automation_settings
    DROP CONSTRAINT IF EXISTS assistant_automation_settings_max_ai_calls_per_hour_check;

ALTER TABLE assistant_automation_settings
    ALTER COLUMN max_ai_calls_per_hour SET DEFAULT 0;

UPDATE assistant_automation_settings
SET max_ai_calls_per_hour = 0
WHERE max_ai_calls_per_hour <> 0;

ALTER TABLE assistant_automation_settings
    ADD CONSTRAINT assistant_automation_settings_max_ai_calls_per_hour_check
    CHECK (max_ai_calls_per_hour >= 0);
