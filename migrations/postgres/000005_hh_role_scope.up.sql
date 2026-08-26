-- Phase 1 product scope: official HH professional role allowlist.
-- Canonical policy: apps/ingest/internal/hh/role_policy.go.

UPDATE roles AS r
SET family = CASE ra.pattern
    WHEN '96' THEN 'software_development'
    WHEN '104' THEN 'software_development'
    WHEN '148' THEN 'analytics'
    WHEN '150' THEN 'analytics'
    WHEN '156' THEN 'analytics'
    WHEN '164' THEN 'analytics'
    WHEN '124' THEN 'quality_assurance'
END
FROM role_aliases AS ra
WHERE ra.role_id = r.id
  AND ra.source = 'hh'
  AND ra.pattern IN ('96', '104', '148', '150', '156', '164', '124');

-- Reconcile a deterministic primary canonical role for allowed historical rows.
WITH allowed_aliases AS (
    SELECT ra.role_id, ra.pattern,
        CASE ra.pattern
            WHEN '96' THEN 10 WHEN '104' THEN 20 WHEN '148' THEN 30
            WHEN '150' THEN 40 WHEN '156' THEN 50 WHEN '164' THEN 60
            WHEN '124' THEN 70
        END AS priority
    FROM role_aliases AS ra
    WHERE ra.source = 'hh'
      AND ra.pattern IN ('96', '104', '148', '150', '156', '164', '124')
), resolved AS (
    SELECT v.id, chosen.role_id
    FROM vacancies AS v
    CROSS JOIN LATERAL (
        SELECT aa.role_id
        FROM jsonb_array_elements(COALESCE(v.raw_payload -> 'professional_roles', '[]'::jsonb)) AS pr
        JOIN allowed_aliases AS aa ON aa.pattern = pr ->> 'id'
        ORDER BY aa.priority
        LIMIT 1
    ) AS chosen
    WHERE v.source = 'hh'
)
UPDATE vacancies AS v
SET role_id = resolved.role_id, updated_at = now()
FROM resolved
WHERE v.id = resolved.id
  AND v.role_id IS DISTINCT FROM resolved.role_id;

-- Preserve history but remove out-of-scope or unresolved HH vacancies from the
-- active product. Employers and skills are intentionally retained.
UPDATE vacancies AS v
SET is_active = false, updated_at = now()
WHERE v.source = 'hh'
  AND v.is_active
  AND NOT EXISTS (
      SELECT 1
      FROM jsonb_array_elements(COALESCE(v.raw_payload -> 'professional_roles', '[]'::jsonb)) AS pr
      WHERE pr ->> 'id' IN ('96', '104', '148', '150', '156', '164', '124')
  );
