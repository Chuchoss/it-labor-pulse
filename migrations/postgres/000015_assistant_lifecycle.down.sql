DROP TABLE IF EXISTS assistant_runs;
DROP INDEX IF EXISTS vacancy_preferences_active;
ALTER TABLE vacancy_preferences DROP COLUMN IF EXISTS archived_at;
