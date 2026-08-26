-- Phase 1: official daily FX rates and source-neutral vacancy links.
CREATE TABLE fx_rates (
    rate_date       DATE NOT NULL,
    provider        TEXT NOT NULL,
    base_currency   CHAR(3) NOT NULL DEFAULT 'RUB',
    quote_currency  CHAR(3) NOT NULL,
    rub_per_unit    NUMERIC(20, 10) NOT NULL,
    nominal         INTEGER NOT NULL,
    source_value    NUMERIC(20, 10) NOT NULL,
    fetched_at      TIMESTAMPTZ NOT NULL,
    source_revision TEXT NULL,
    provenance      TEXT NOT NULL,
    PRIMARY KEY (provider, rate_date, base_currency, quote_currency),
    CONSTRAINT fx_rates_provider_check CHECK (provider ~ '^[a-z][a-z0-9_-]{1,31}$'),
    CONSTRAINT fx_rates_currency_check CHECK (
        base_currency = upper(base_currency)
        AND quote_currency = upper(quote_currency)
        AND base_currency <> quote_currency
    ),
    CONSTRAINT fx_rates_base_check CHECK (base_currency = 'RUB'),
    CONSTRAINT fx_rates_values_check CHECK (
        nominal > 0 AND source_value > 0 AND rub_per_unit > 0
    ),
    CONSTRAINT fx_rates_unit_check CHECK (
        abs(rub_per_unit - source_value / nominal) < 0.0000000001
    )
);

CREATE INDEX idx_fx_rates_latest
    ON fx_rates (quote_currency, provider, rate_date DESC)
    INCLUDE (rub_per_unit, nominal, fetched_at);

CREATE TABLE fx_sync_runs (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider       TEXT NOT NULL,
    requested_from DATE NOT NULL,
    requested_to   DATE NOT NULL,
    status         TEXT NOT NULL,
    fetched_dates  INTEGER NOT NULL DEFAULT 0,
    upserted_rates INTEGER NOT NULL DEFAULT 0,
    started_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at    TIMESTAMPTZ NULL,
    error_category TEXT NULL,
    CONSTRAINT fx_sync_runs_range_check CHECK (requested_from <= requested_to),
    CONSTRAINT fx_sync_runs_status_check CHECK (status IN ('running', 'success', 'failed')),
    CONSTRAINT fx_sync_runs_counts_check CHECK (fetched_dates >= 0 AND upserted_rates >= 0),
    CONSTRAINT fx_sync_runs_finished_check CHECK (
        (status = 'running' AND finished_at IS NULL)
        OR (status <> 'running' AND finished_at IS NOT NULL)
    )
);

ALTER TABLE vacancies
    ADD COLUMN source_url TEXT NULL,
    ADD COLUMN salary_from_rub_net NUMERIC(12, 2) NULL,
    ADD COLUMN salary_to_rub_net NUMERIC(12, 2) NULL,
    ADD COLUMN salary_rate_date DATE NULL,
    ADD COLUMN salary_rate_provider TEXT NULL,
    ADD CONSTRAINT vacancies_source_url_length_check CHECK (
        source_url IS NULL OR length(source_url) <= 2048
    ),
    ADD CONSTRAINT vacancies_source_url_scheme_check CHECK (
        source_url IS NULL OR source_url ~ '^https?://'
    ),
    ADD CONSTRAINT vacancies_salary_rate_check CHECK (
        (salary_rate_date IS NULL AND salary_rate_provider IS NULL)
        OR (salary_rate_date IS NOT NULL AND salary_rate_provider IS NOT NULL)
    );

ALTER TABLE ingest_cycle_observations
    ADD COLUMN salary_rate_date DATE NULL,
    ADD COLUMN salary_rate_provider TEXT NULL,
    ADD CONSTRAINT ingest_observations_salary_rate_check CHECK (
        (salary_rate_date IS NULL AND salary_rate_provider IS NULL)
        OR (salary_rate_date IS NOT NULL AND salary_rate_provider IS NOT NULL)
    );

-- Existing RUB rows already use canonical net mid semantics.
UPDATE vacancies
SET salary_from_rub_net = salary_from,
    salary_to_rub_net = salary_to
WHERE trim(salary_currency) = 'RUB';

-- Official deterministic HH URL form; numeric ids only.
UPDATE vacancies
SET source_url = 'https://hh.ru/vacancy/' || external_id
WHERE source = 'hh'
  AND external_id ~ '^[0-9]+$'
  AND source_url IS NULL;

COMMENT ON COLUMN vacancies.source_url IS
    'Adapter-provided official source URL; never derived by the browser.';
COMMENT ON COLUMN vacancies.salary_mid IS
    'Canonical RUB/net midpoint when official FX is available; NULL otherwise.';
COMMENT ON TABLE fx_rates IS
    'Official daily reference rates normalized as RUB per one quote unit.';
