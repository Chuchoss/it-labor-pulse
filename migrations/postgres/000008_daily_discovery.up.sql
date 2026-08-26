-- Phase 1 daily discovery: trustworthy search-only observations and v2 snapshots.

ALTER TABLE ingest_cycles DROP CONSTRAINT ingest_cycles_scope_check;
ALTER TABLE ingest_cycles
    ADD CONSTRAINT ingest_cycles_scope_check
        CHECK (scope IN ('all_it', 'daily_discovery')),
    ADD COLUMN cycle_date DATE NULL,
    ADD COLUMN cutoff_at TIMESTAMPTZ NULL,
    ADD COLUMN expected_pages INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN completed_pages INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN method_version TEXT NULL,
    ADD COLUMN failed_at TIMESTAMPTZ NULL,
    ADD CONSTRAINT ingest_cycles_page_count_check
        CHECK (expected_pages >= 0 AND completed_pages >= 0
            AND completed_pages <= expected_pages),
    ADD CONSTRAINT ingest_cycles_discovery_check
        CHECK (
            scope <> 'daily_discovery'
            OR (
                cycle_date IS NOT NULL
                AND cutoff_at IS NOT NULL
                AND method_version = 'vacancy_demand_v2'
            )
        );

CREATE UNIQUE INDEX idx_ingest_cycles_daily_discovery
    ON ingest_cycles (source, cycle_date, method_version)
    WHERE scope = 'daily_discovery';

CREATE TABLE ingest_cycle_partitions (
    cycle_id             UUID NOT NULL REFERENCES ingest_cycles (id) ON DELETE CASCADE,
    partition_key        TEXT NOT NULL,
    professional_role_id TEXT NOT NULL,
    area                 TEXT NOT NULL,
    date_from             TIMESTAMPTZ NULL,
    date_to               TIMESTAMPTZ NOT NULL,
    expected_pages        INTEGER NOT NULL,
    next_page             INTEGER NOT NULL DEFAULT 0,
    status                TEXT NOT NULL DEFAULT 'pending',
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (cycle_id, partition_key),
    CONSTRAINT ingest_cycle_partitions_pages_check
        CHECK (expected_pages >= 0 AND next_page >= 0 AND next_page <= expected_pages),
    CONSTRAINT ingest_cycle_partitions_status_check
        CHECK (status IN ('pending', 'running', 'complete', 'failed')),
    CONSTRAINT ingest_cycle_partitions_completion_check
        CHECK (status <> 'complete' OR next_page = expected_pages)
);

CREATE INDEX idx_ingest_cycle_partitions_resume
    ON ingest_cycle_partitions (cycle_id, status, partition_key);

CREATE TABLE ingest_cycle_observations (
    cycle_id                 UUID NOT NULL REFERENCES ingest_cycles (id) ON DELETE CASCADE,
    source                   TEXT NOT NULL REFERENCES sources (code),
    external_id              TEXT NOT NULL,
    published_at             TIMESTAMPTZ NOT NULL,
    external_region_id       TEXT NULL,
    external_region_name     TEXT NULL,
    primary_role_external_id TEXT NOT NULL,
    role_group               TEXT NOT NULL,
    external_role_ids        TEXT[] NOT NULL DEFAULT '{}',
    salary_from              NUMERIC(12, 2) NULL,
    salary_to                NUMERIC(12, 2) NULL,
    salary_currency          CHAR(3) NULL,
    salary_gross             BOOLEAN NULL,
    salary_mid_rub_net       NUMERIC(12, 2) NULL,
    salary_eligible          BOOLEAN NOT NULL DEFAULT false,
    observed_at              TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (cycle_id, source, external_id),
    CONSTRAINT ingest_cycle_observations_role_group_check
        CHECK (role_group IN ('software_development', 'analytics', 'quality_assurance')),
    CONSTRAINT ingest_cycle_observations_external_id_check
        CHECK (external_id <> ''),
    CONSTRAINT ingest_cycle_observations_salary_check
        CHECK (
            (salary_eligible AND salary_mid_rub_net BETWEEN 10000 AND 2000000)
            OR (NOT salary_eligible AND salary_mid_rub_net IS NULL)
        )
);

CREATE INDEX idx_ingest_cycle_observations_aggregate
    ON ingest_cycle_observations (cycle_id, role_group, external_region_id)
    INCLUDE (published_at, salary_mid_rub_net, salary_eligible);

ALTER TABLE vacancy_demand_daily
    ADD COLUMN salary_method TEXT NOT NULL DEFAULT 'hydrated_detail_v1',
    ADD COLUMN salary_coverage NUMERIC(7, 6) NOT NULL DEFAULT 0,
    ADD CONSTRAINT vacancy_demand_daily_salary_coverage_check
        CHECK (salary_coverage BETWEEN 0 AND 1);

COMMENT ON TABLE ingest_cycle_observations IS
    'Minimal HH search observations. Keep at least 35 days; cleanup only after successful snapshots.';
COMMENT ON COLUMN ingest_cycle_observations.external_role_ids IS
    'Official per-vacancy HH professional roles; partition defaults are never used for attribution.';
