ALTER TABLE vacancy_demand_daily
    DROP CONSTRAINT vacancy_demand_daily_salary_coverage_check,
    DROP COLUMN salary_coverage,
    DROP COLUMN salary_method;

DROP TABLE IF EXISTS ingest_cycle_observations;
DROP TABLE IF EXISTS ingest_cycle_partitions;

DROP INDEX IF EXISTS idx_ingest_cycles_daily_discovery;

ALTER TABLE ingest_cycles
    DROP CONSTRAINT ingest_cycles_discovery_check,
    DROP CONSTRAINT ingest_cycles_page_count_check,
    DROP COLUMN failed_at,
    DROP COLUMN method_version,
    DROP COLUMN completed_pages,
    DROP COLUMN expected_pages,
    DROP COLUMN cutoff_at,
    DROP COLUMN cycle_date;

ALTER TABLE ingest_cycles DROP CONSTRAINT ingest_cycles_scope_check;
ALTER TABLE ingest_cycles
    ADD CONSTRAINT ingest_cycles_scope_check CHECK (scope = 'all_it');
