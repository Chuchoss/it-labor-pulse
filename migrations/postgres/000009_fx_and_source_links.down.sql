ALTER TABLE ingest_cycle_observations
    DROP CONSTRAINT IF EXISTS ingest_observations_salary_rate_check,
    DROP COLUMN IF EXISTS salary_rate_provider,
    DROP COLUMN IF EXISTS salary_rate_date;

-- Restore the v8 meaning: salary_mid in the original source currency (net estimate).
UPDATE vacancies
SET salary_mid = round((
    coalesce((salary_from + salary_to) / 2, salary_from, salary_to)
    * CASE WHEN salary_gross THEN 0.87 ELSE 1 END
)::numeric, 2)
WHERE salary_rate_date IS NOT NULL
  AND trim(salary_currency) <> 'RUB';

ALTER TABLE vacancies
    DROP CONSTRAINT IF EXISTS vacancies_salary_rate_check,
    DROP CONSTRAINT IF EXISTS vacancies_source_url_scheme_check,
    DROP CONSTRAINT IF EXISTS vacancies_source_url_length_check,
    DROP COLUMN IF EXISTS salary_rate_provider,
    DROP COLUMN IF EXISTS salary_rate_date,
    DROP COLUMN IF EXISTS salary_to_rub_net,
    DROP COLUMN IF EXISTS salary_from_rub_net,
    DROP COLUMN IF EXISTS source_url;

DROP TABLE IF EXISTS fx_sync_runs;
DROP TABLE IF EXISTS fx_rates;
