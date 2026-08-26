-- Preserve the first source collection time independently from mutable collected_at.
ALTER TABLE vacancies
    ADD COLUMN first_observed_at TIMESTAMPTZ NULL;

UPDATE vacancies
SET first_observed_at = created_at
WHERE first_observed_at IS NULL;

ALTER TABLE vacancies
    ALTER COLUMN first_observed_at SET NOT NULL;

CREATE INDEX idx_vacancies_first_observed_at
    ON vacancies (first_observed_at DESC);

COMMENT ON COLUMN vacancies.first_observed_at IS
    'UTC time when this source vacancy was first observed by ingest; immutable after insert.';
