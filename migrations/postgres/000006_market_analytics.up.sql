-- Phase 1 market analytics: durable full-cycle markers and reproducible snapshots.

CREATE TABLE ingest_cycles (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source               TEXT NOT NULL REFERENCES sources (code),
    scope                TEXT NOT NULL,
    scope_hash           CHAR(64) NOT NULL,
    cycle_end            TIMESTAMPTZ NOT NULL,
    status               TEXT NOT NULL DEFAULT 'running',
    partition_count      INTEGER NOT NULL,
    completed_partitions INTEGER NOT NULL DEFAULT 0,
    started_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at         TIMESTAMPTZ NULL,
    CONSTRAINT ingest_cycles_scope_check
        CHECK (scope = 'all_it'),
    CONSTRAINT ingest_cycles_status_check
        CHECK (status IN ('running', 'complete', 'failed')),
    CONSTRAINT ingest_cycles_partition_count_check
        CHECK (partition_count >= 0),
    CONSTRAINT ingest_cycles_completed_partitions_check
        CHECK (completed_partitions >= 0 AND completed_partitions <= partition_count),
    CONSTRAINT ingest_cycles_completion_check
        CHECK (
            (status = 'complete' AND completed_at IS NOT NULL
                AND completed_partitions = partition_count)
            OR (status <> 'complete' AND completed_at IS NULL)
        ),
    UNIQUE (source, scope_hash, cycle_end)
);

CREATE INDEX idx_ingest_cycles_complete_source_end
    ON ingest_cycles (source, cycle_end DESC)
    WHERE status = 'complete';

CREATE TABLE analytics_runs (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_type            TEXT NOT NULL,
    target_period_start DATE NOT NULL,
    source              TEXT NOT NULL REFERENCES sources (code),
    source_cycle_id     UUID NULL REFERENCES ingest_cycles (id),
    status              TEXT NOT NULL,
    method_version      TEXT NOT NULL,
    started_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at         TIMESTAMPTZ NULL,
    row_count           INTEGER NOT NULL DEFAULT 0,
    source_day_count    SMALLINT NOT NULL DEFAULT 0,
    error_category      TEXT NULL,
    error_message       TEXT NULL,
    CONSTRAINT analytics_runs_type_check
        CHECK (run_type IN ('daily_snapshot', 'weekly_rollup')),
    CONSTRAINT analytics_runs_status_check
        CHECK (status IN ('running', 'success', 'skipped', 'failed')),
    CONSTRAINT analytics_runs_row_count_check CHECK (row_count >= 0),
    CONSTRAINT analytics_runs_source_day_count_check
        CHECK (source_day_count BETWEEN 0 AND 7),
    CONSTRAINT analytics_runs_cycle_check
        CHECK (
            (run_type = 'daily_snapshot')
            OR (run_type = 'weekly_rollup' AND source_cycle_id IS NULL)
        ),
    UNIQUE (run_type, target_period_start, source, method_version)
);

CREATE INDEX idx_analytics_runs_source_status_period
    ON analytics_runs (source, status, target_period_start DESC);

CREATE TABLE vacancy_demand_daily (
    snapshot_date          DATE NOT NULL,
    source                 TEXT NOT NULL REFERENCES sources (code),
    role_group             TEXT NOT NULL,
    aggregation_level      TEXT NOT NULL,
    region_id              UUID NULL REFERENCES regions (id),
    active_count           INTEGER NOT NULL,
    published_count        INTEGER NOT NULL,
    vacancies_with_salary  INTEGER NOT NULL,
    median_salary_rub_net  NUMERIC(12, 2) NULL,
    cycle_complete         BOOLEAN NOT NULL,
    source_cycle_id        UUID NOT NULL REFERENCES ingest_cycles (id),
    analytics_run_id       UUID NOT NULL REFERENCES analytics_runs (id),
    method_version         TEXT NOT NULL,
    observed_at            TIMESTAMPTZ NOT NULL,
    CONSTRAINT vacancy_demand_daily_role_group_check
        CHECK (role_group IN ('software_development', 'analytics', 'quality_assurance')),
    CONSTRAINT vacancy_demand_daily_level_check
        CHECK (aggregation_level IN ('all_regions', 'region')),
    CONSTRAINT vacancy_demand_daily_region_check
        CHECK (
            (aggregation_level = 'all_regions' AND region_id IS NULL)
            OR (aggregation_level = 'region' AND region_id IS NOT NULL)
        ),
    CONSTRAINT vacancy_demand_daily_counts_check
        CHECK (
            active_count >= 0
            AND published_count >= 0
            AND vacancies_with_salary >= 0
            AND vacancies_with_salary <= active_count
        ),
    CONSTRAINT vacancy_demand_daily_salary_check
        CHECK (median_salary_rub_net IS NULL OR median_salary_rub_net BETWEEN 10000 AND 2000000),
    CONSTRAINT vacancy_demand_daily_complete_check CHECK (cycle_complete),
    UNIQUE NULLS NOT DISTINCT (
        snapshot_date, source, role_group, aggregation_level, region_id, method_version
    )
);

CREATE INDEX idx_vacancy_demand_daily_series
    ON vacancy_demand_daily (
        source, role_group, aggregation_level, region_id, snapshot_date
    )
    INCLUDE (
        active_count, published_count, vacancies_with_salary,
        median_salary_rub_net, observed_at, method_version
    );

CREATE TABLE vacancy_demand_weekly (
    week_start             DATE NOT NULL,
    source                 TEXT NOT NULL REFERENCES sources (code),
    role_group             TEXT NOT NULL,
    aggregation_level      TEXT NOT NULL,
    region_id              UUID NULL REFERENCES regions (id),
    active_count           INTEGER NOT NULL,
    published_count        INTEGER NOT NULL,
    vacancies_with_salary  INTEGER NOT NULL,
    median_salary_rub_net  NUMERIC(12, 2) NULL,
    source_daily_count     SMALLINT NOT NULL,
    complete               BOOLEAN NOT NULL,
    analytics_run_id       UUID NOT NULL REFERENCES analytics_runs (id),
    method_version         TEXT NOT NULL,
    observed_at            TIMESTAMPTZ NOT NULL,
    CONSTRAINT vacancy_demand_weekly_monday_check
        CHECK (extract(isodow FROM week_start) = 1),
    CONSTRAINT vacancy_demand_weekly_role_group_check
        CHECK (role_group IN ('software_development', 'analytics', 'quality_assurance')),
    CONSTRAINT vacancy_demand_weekly_level_check
        CHECK (aggregation_level IN ('all_regions', 'region')),
    CONSTRAINT vacancy_demand_weekly_region_check
        CHECK (
            (aggregation_level = 'all_regions' AND region_id IS NULL)
            OR (aggregation_level = 'region' AND region_id IS NOT NULL)
        ),
    CONSTRAINT vacancy_demand_weekly_counts_check
        CHECK (
            active_count >= 0
            AND published_count >= 0
            AND vacancies_with_salary >= 0
        ),
    CONSTRAINT vacancy_demand_weekly_source_days_check
        CHECK (source_daily_count BETWEEN 1 AND 7),
    CONSTRAINT vacancy_demand_weekly_complete_check
        CHECK (complete = (source_daily_count = 7)),
    CONSTRAINT vacancy_demand_weekly_salary_check
        CHECK (median_salary_rub_net IS NULL OR median_salary_rub_net BETWEEN 10000 AND 2000000),
    UNIQUE NULLS NOT DISTINCT (
        week_start, source, role_group, aggregation_level, region_id, method_version
    )
);

CREATE INDEX idx_vacancy_demand_weekly_series
    ON vacancy_demand_weekly (
        source, role_group, aggregation_level, region_id, week_start
    )
    INCLUDE (
        active_count, published_count, vacancies_with_salary,
        median_salary_rub_net, source_daily_count, complete, method_version
    );
