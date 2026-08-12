-- Phase 1: reference dictionaries (regions, roles, skills, employers)

CREATE TABLE regions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    code         TEXT NOT NULL,
    name         TEXT NOT NULL,
    country_code CHAR(2) NOT NULL DEFAULT 'RU',
    parent_id    UUID NULL REFERENCES regions (id),
    is_active    BOOLEAN NOT NULL DEFAULT true,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (country_code, code)
);

CREATE INDEX idx_regions_parent_id ON regions (parent_id);
CREATE INDEX idx_regions_is_active ON regions (is_active) WHERE is_active;

CREATE TABLE region_external_ids (
    source      TEXT NOT NULL REFERENCES sources (code),
    external_id TEXT NOT NULL,
    region_id   UUID NOT NULL REFERENCES regions (id),
    PRIMARY KEY (source, external_id)
);

CREATE INDEX idx_region_external_ids_region_id ON region_external_ids (region_id);

CREATE TABLE roles (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug      TEXT NOT NULL UNIQUE,
    title     TEXT NOT NULL,
    family    TEXT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true
);

CREATE TABLE role_aliases (
    id      BIGSERIAL PRIMARY KEY,
    role_id UUID NOT NULL REFERENCES roles (id),
    pattern TEXT NOT NULL,
    source  TEXT NULL REFERENCES sources (code),
    UNIQUE (source, pattern)
);

CREATE INDEX idx_role_aliases_role_id ON role_aliases (role_id);

CREATE TABLE skills (
    id        UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug      TEXT NOT NULL UNIQUE,
    name      TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true
);

CREATE TABLE skill_aliases (
    id       BIGSERIAL PRIMARY KEY,
    skill_id UUID NOT NULL REFERENCES skills (id),
    pattern  TEXT NOT NULL,
    source   TEXT NULL REFERENCES sources (code),
    UNIQUE (source, pattern)
);

CREATE INDEX idx_skill_aliases_skill_id ON skill_aliases (skill_id);

CREATE TABLE employers (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source      TEXT NOT NULL REFERENCES sources (code),
    external_id TEXT NOT NULL,
    name        TEXT NOT NULL,
    is_active   BOOLEAN NOT NULL DEFAULT true,
    UNIQUE (source, external_id)
);
