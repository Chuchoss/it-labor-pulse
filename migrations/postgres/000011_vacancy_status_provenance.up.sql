-- Preserve source availability transitions and their complete-cycle provenance.
ALTER TABLE vacancies
    ADD COLUMN last_seen_at TIMESTAMPTZ NULL,
    ADD COLUMN last_seen_cycle_id UUID NULL REFERENCES ingest_cycles (id),
    ADD COLUMN deactivated_at TIMESTAMPTZ NULL,
    ADD COLUMN deactivation_reason TEXT NULL;

CREATE INDEX idx_vacancies_last_seen_cycle
    ON vacancies (source, last_seen_cycle_id)
    WHERE last_seen_cycle_id IS NOT NULL;

COMMENT ON COLUMN vacancies.last_seen_cycle_id IS
    'Complete source discovery cycle that verified the latest current availability.';
COMMENT ON COLUMN vacancies.deactivation_reason IS
    'Machine-readable reason for inactive status, for example missing_from_complete_cycle or detail_not_found.';
