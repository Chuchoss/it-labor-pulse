-- Data deactivation is intentionally not reversed: previous active state
-- cannot be reconstructed safely. A subsequent allowed-role ingest can
-- reactivate only vacancies that pass the current policy.

UPDATE roles AS r
SET family = 'it'
FROM role_aliases AS ra
WHERE ra.role_id = r.id
  AND ra.source = 'hh'
  AND ra.pattern IN ('96', '104', '148', '150', '156', '164', '124')
  AND r.family IN ('software_development', 'analytics', 'quality_assurance');
