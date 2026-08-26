-- Keep direct legacy inserts compatible while recording their observation time.
ALTER TABLE vacancies
    ALTER COLUMN first_observed_at SET DEFAULT now();
