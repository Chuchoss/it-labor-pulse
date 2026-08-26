DROP INDEX IF EXISTS idx_vacancies_first_observed_at;

ALTER TABLE vacancies
    DROP COLUMN IF EXISTS first_observed_at;
