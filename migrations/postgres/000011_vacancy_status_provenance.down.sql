DROP INDEX IF EXISTS idx_vacancies_last_seen_cycle;

ALTER TABLE vacancies
    DROP COLUMN IF EXISTS deactivation_reason,
    DROP COLUMN IF EXISTS deactivated_at,
    DROP COLUMN IF EXISTS last_seen_cycle_id,
    DROP COLUMN IF EXISTS last_seen_at;
