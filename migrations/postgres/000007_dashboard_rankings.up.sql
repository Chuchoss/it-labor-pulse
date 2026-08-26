-- Phase 1 dashboard rankings: explicit product scopes and language taxonomy.

ALTER TABLE skills
    ADD COLUMN skill_kind TEXT NOT NULL DEFAULT 'other',
    ADD CONSTRAINT skills_kind_check CHECK (
        skill_kind IN (
            'programming_language', 'query_language', 'markup',
            'shell', 'platform_language', 'framework', 'database', 'tool', 'other'
        )
    );

DELETE FROM skill_aliases newer
USING skill_aliases older
WHERE newer.id > older.id
  AND newer.pattern = older.pattern
  AND COALESCE(newer.source, '') = COALESCE(older.source, '');

CREATE UNIQUE INDEX idx_skill_aliases_source_pattern_canonical
    ON skill_aliases (COALESCE(source, ''), pattern);

CREATE TABLE role_scopes (
    role_id UUID NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    scope TEXT NOT NULL,
    PRIMARY KEY (role_id, scope),
    CONSTRAINT role_scopes_scope_check CHECK (scope IN ('vacancy_listing', 'management_analytics'))
);

CREATE TABLE vacancy_role_scopes (
    vacancy_id UUID NOT NULL REFERENCES vacancies (id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles (id),
    scope TEXT NOT NULL,
    PRIMARY KEY (vacancy_id, role_id, scope),
    CONSTRAINT vacancy_role_scopes_scope_check CHECK (scope IN ('vacancy_listing', 'management_analytics'))
);

CREATE INDEX idx_vacancy_role_scopes_rank
    ON vacancy_role_scopes (scope, role_id, vacancy_id);

INSERT INTO role_scopes (role_id, scope)
SELECT DISTINCT ra.role_id, 'vacancy_listing'
FROM role_aliases ra
WHERE ra.source = 'hh'
  AND ra.pattern IN ('96', '104', '148', '150', '156', '164', '124')
ON CONFLICT DO NOTHING;

-- Official HH IT category roles approved for the separate management ranking.
WITH policy(external_id, title, family) AS (
    VALUES
        ('10', 'Аналитик', 'it_management'),
        ('36', 'Директор по информационным технологиям (CIO)', 'it_management'),
        ('73', 'Менеджер продукта', 'it_management'),
        ('104', 'Руководитель группы разработки', 'software_development'),
        ('107', 'Руководитель проектов', 'it_management'),
        ('125', 'Технический директор (CTO)', 'it_management'),
        ('148', 'Системный аналитик', 'analytics'),
        ('150', 'Бизнес-аналитик', 'analytics'),
        ('156', 'BI-аналитик, аналитик данных', 'analytics'),
        ('157', 'Руководитель отдела аналитики', 'it_management'),
        ('164', 'Продуктовый аналитик', 'analytics')
), inserted AS (
    INSERT INTO roles (slug, title, family, is_active)
    SELECT 'hh-' || family || '-' || external_id, title, family, true
    FROM policy
    ON CONFLICT (slug) DO UPDATE
      SET title = EXCLUDED.title, is_active = true
    RETURNING id, slug
)
INSERT INTO role_aliases (role_id, pattern, source)
SELECT i.id, p.external_id, 'hh'
FROM inserted i
JOIN policy p ON i.slug = 'hh-' || p.family || '-' || p.external_id
ON CONFLICT (source, pattern) DO UPDATE SET role_id = EXCLUDED.role_id;

INSERT INTO role_scopes (role_id, scope)
SELECT ra.role_id, 'management_analytics'
FROM role_aliases ra
WHERE ra.source = 'hh'
  AND ra.pattern IN ('10', '36', '73', '104', '107', '125', '148', '150', '156', '157', '164')
ON CONFLICT DO NOTHING;

INSERT INTO vacancy_role_scopes (vacancy_id, role_id, scope)
SELECT DISTINCT v.id, ra.role_id, rs.scope
FROM vacancies v
CROSS JOIN LATERAL jsonb_array_elements(
    COALESCE(v.raw_payload -> 'professional_roles', '[]'::jsonb)
) pr
JOIN role_aliases ra ON ra.source = v.source AND ra.pattern = pr ->> 'id'
JOIN role_scopes rs ON rs.role_id = ra.role_id
WHERE v.source = 'hh'
ON CONFLICT DO NOTHING;

-- Canonical programming languages. SQL, HTML/CSS and Bash are deliberately
-- classified but excluded from the programming-language ranking. 1C is kept
-- as a platform language and is also excluded from that strict ranking.
WITH taxonomy(slug, name, kind, aliases) AS (
    VALUES
      ('go', 'Go', 'programming_language', ARRAY['go','golang']),
      ('javascript', 'JavaScript', 'programming_language', ARRAY['javascript','js','ecmascript']),
      ('typescript', 'TypeScript', 'programming_language', ARRAY['typescript','ts']),
      ('python', 'Python', 'programming_language', ARRAY['python','python3']),
      ('java', 'Java', 'programming_language', ARRAY['java']),
      ('c-sharp', 'C#', 'programming_language', ARRAY['c#','c-sharp','c-sharp-.net','csharp']),
      ('c-plus-plus', 'C++', 'programming_language', ARRAY['c++','cpp','c-plus-plus']),
      ('c', 'C', 'programming_language', ARRAY['c']),
      ('php', 'PHP', 'programming_language', ARRAY['php']),
      ('kotlin', 'Kotlin', 'programming_language', ARRAY['kotlin']),
      ('swift', 'Swift', 'programming_language', ARRAY['swift']),
      ('ruby', 'Ruby', 'programming_language', ARRAY['ruby']),
      ('rust', 'Rust', 'programming_language', ARRAY['rust']),
      ('scala', 'Scala', 'programming_language', ARRAY['scala']),
      ('dart', 'Dart', 'programming_language', ARRAY['dart']),
      ('objective-c', 'Objective-C', 'programming_language', ARRAY['objective-c','objective-c++']),
      ('r', 'R', 'programming_language', ARRAY['r']),
      ('lua', 'Lua', 'programming_language', ARRAY['lua']),
      ('perl', 'Perl', 'programming_language', ARRAY['perl']),
      ('groovy', 'Groovy', 'programming_language', ARRAY['groovy']),
      ('solidity', 'Solidity', 'programming_language', ARRAY['solidity']),
      ('delphi', 'Delphi', 'programming_language', ARRAY['delphi','object-pascal','pascal']),
      ('sql', 'SQL', 'query_language', ARRAY['sql','pl/sql','pl-pgsql','t-sql']),
      ('html', 'HTML', 'markup', ARRAY['html','html5']),
      ('css', 'CSS', 'markup', ARRAY['css','css3']),
      ('bash', 'Bash', 'shell', ARRAY['bash','shell','shell-script']),
      ('1c', '1C', 'platform_language', ARRAY['1c','1с','1c-enterprise'])
), canonical AS (
    INSERT INTO skills (slug, name, skill_kind, is_active)
    SELECT slug, name, kind, true FROM taxonomy
    ON CONFLICT (slug) DO UPDATE
      SET name = EXCLUDED.name, skill_kind = EXCLUDED.skill_kind, is_active = true
    RETURNING id, slug
), alias_rows AS (
    SELECT c.id AS canonical_id, unnest(t.aliases) AS alias
    FROM taxonomy t JOIN canonical c USING (slug)
), remap AS (
    SELECT s.id AS old_id, a.canonical_id
    FROM skills s
    JOIN alias_rows a ON lower(s.slug) = a.alias
    WHERE s.id <> a.canonical_id
)
INSERT INTO vacancy_skills (vacancy_id, skill_id)
SELECT DISTINCT vs.vacancy_id, r.canonical_id
FROM vacancy_skills vs JOIN remap r ON r.old_id = vs.skill_id
ON CONFLICT DO NOTHING;

WITH taxonomy(slug, aliases) AS (
    VALUES
      ('go', ARRAY['go','golang']), ('javascript', ARRAY['javascript','js','ecmascript']),
      ('typescript', ARRAY['typescript','ts']), ('python', ARRAY['python','python3']),
      ('java', ARRAY['java']), ('c-sharp', ARRAY['c#','c-sharp','c-sharp-.net','csharp']),
      ('c-plus-plus', ARRAY['c++','cpp','c-plus-plus']), ('c', ARRAY['c']),
      ('php', ARRAY['php']), ('kotlin', ARRAY['kotlin']), ('swift', ARRAY['swift']),
      ('ruby', ARRAY['ruby']), ('rust', ARRAY['rust']), ('scala', ARRAY['scala']),
      ('dart', ARRAY['dart']), ('objective-c', ARRAY['objective-c','objective-c++']),
      ('r', ARRAY['r']), ('lua', ARRAY['lua']), ('perl', ARRAY['perl']),
      ('groovy', ARRAY['groovy']), ('solidity', ARRAY['solidity']),
      ('delphi', ARRAY['delphi','object-pascal','pascal']), ('sql', ARRAY['sql','pl/sql','pl-pgsql','t-sql']),
      ('html', ARRAY['html','html5']), ('css', ARRAY['css','css3']),
      ('bash', ARRAY['bash','shell','shell-script']), ('1c', ARRAY['1c','1с','1c-enterprise'])
), aliases AS (
    SELECT s.id, unnest(t.aliases) AS pattern
    FROM taxonomy t JOIN skills s USING (slug)
)
INSERT INTO skill_aliases (skill_id, pattern, source)
SELECT id, pattern, NULL FROM aliases
ON CONFLICT DO NOTHING;

WITH taxonomy(slug, aliases) AS (
    VALUES
      ('go', ARRAY['go','golang']), ('javascript', ARRAY['javascript','js','ecmascript']),
      ('typescript', ARRAY['typescript','ts']), ('python', ARRAY['python','python3']),
      ('java', ARRAY['java']), ('c-sharp', ARRAY['c#','c-sharp','c-sharp-.net','csharp']),
      ('c-plus-plus', ARRAY['c++','cpp','c-plus-plus']), ('c', ARRAY['c']),
      ('php', ARRAY['php']), ('kotlin', ARRAY['kotlin']), ('swift', ARRAY['swift']),
      ('ruby', ARRAY['ruby']), ('rust', ARRAY['rust']), ('scala', ARRAY['scala']),
      ('dart', ARRAY['dart']), ('objective-c', ARRAY['objective-c','objective-c++']),
      ('r', ARRAY['r']), ('lua', ARRAY['lua']), ('perl', ARRAY['perl']),
      ('groovy', ARRAY['groovy']), ('solidity', ARRAY['solidity']),
      ('delphi', ARRAY['delphi','object-pascal','pascal']), ('sql', ARRAY['sql','pl/sql','pl-pgsql','t-sql']),
      ('html', ARRAY['html','html5']), ('css', ARRAY['css','css3']),
      ('bash', ARRAY['bash','shell','shell-script']), ('1c', ARRAY['1c','1с','1c-enterprise'])
), canonical AS (
    SELECT s.id, unnest(t.aliases) AS pattern
    FROM taxonomy t JOIN skills s USING (slug)
)
UPDATE skill_aliases sa
SET skill_id = canonical.id
FROM canonical
WHERE sa.source IS NULL AND sa.pattern = canonical.pattern;

-- Remove duplicate non-canonical links after remapping; dictionary rows remain
-- for audit and old foreign keys.
WITH canonical_aliases AS (
    SELECT sa.skill_id AS canonical_id, sa.pattern
    FROM skill_aliases sa
    WHERE sa.source IS NULL
)
DELETE FROM vacancy_skills vs
USING skills old, canonical_aliases ca
WHERE vs.skill_id = old.id
  AND old.id <> ca.canonical_id
  AND lower(old.slug) = ca.pattern;
