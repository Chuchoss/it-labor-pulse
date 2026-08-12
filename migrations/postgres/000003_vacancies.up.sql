-- Phase 1: vacancies + vacancy_skills

CREATE TABLE vacancies (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source           TEXT NOT NULL REFERENCES sources (code),
    external_id      TEXT NOT NULL,
    title            TEXT NOT NULL,
    employer_id      UUID NULL REFERENCES employers (id),
    role_id          UUID NULL REFERENCES roles (id),
    region_id        UUID NULL REFERENCES regions (id),
    salary_from      NUMERIC(12, 2) NULL,
    salary_to        NUMERIC(12, 2) NULL,
    salary_currency  CHAR(3) NULL,
    salary_gross     BOOLEAN NULL,
    salary_mid       NUMERIC(12, 2) NULL,
    description_text TEXT NULL,
    published_at     TIMESTAMPTZ NULL,
    collected_at     TIMESTAMPTZ NOT NULL,
    is_active        BOOLEAN NOT NULL DEFAULT true,
    deleted_at       TIMESTAMPTZ NULL,
    content_hash     BYTEA NULL,
    raw_payload      JSONB NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source, external_id)
);

CREATE INDEX idx_vacancies_role_active
    ON vacancies (role_id, is_active)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_vacancies_region_active
    ON vacancies (region_id, is_active)
    WHERE deleted_at IS NULL;

CREATE INDEX idx_vacancies_published_at
    ON vacancies (published_at DESC);

CREATE INDEX idx_vacancies_is_active
    ON vacancies (is_active)
    WHERE is_active = true;

CREATE INDEX idx_vacancies_salary_mid
    ON vacancies (salary_mid)
    WHERE salary_mid IS NOT NULL AND is_active;

CREATE INDEX idx_vacancies_role_region_published
    ON vacancies (role_id, region_id, published_at DESC)
    WHERE is_active;

CREATE TABLE vacancy_skills (
    vacancy_id UUID NOT NULL REFERENCES vacancies (id) ON DELETE CASCADE,
    skill_id   UUID NOT NULL REFERENCES skills (id),
    PRIMARY KEY (vacancy_id, skill_id)
);

CREATE INDEX idx_vacancy_skills_skill_vacancy
    ON vacancy_skills (skill_id, vacancy_id);
