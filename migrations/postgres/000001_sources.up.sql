-- Phase 1: job board source registry
CREATE TABLE sources (
    code       TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    is_active  BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO sources (code, name, is_active)
VALUES ('hh', 'HeadHunter', true);
