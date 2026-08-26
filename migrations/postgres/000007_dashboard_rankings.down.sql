DROP TABLE IF EXISTS vacancy_role_scopes;
DROP TABLE IF EXISTS role_scopes;

DROP INDEX IF EXISTS idx_skill_aliases_source_pattern_canonical;

ALTER TABLE skills
    DROP CONSTRAINT IF EXISTS skills_kind_check,
    DROP COLUMN IF EXISTS skill_kind;
