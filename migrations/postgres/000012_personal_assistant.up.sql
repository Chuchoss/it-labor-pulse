-- Phase 4-shaped personal assistant foundation. Public vacancy facts remain separate.
CREATE TABLE assistant_users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    external_subject TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE vacancy_preferences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES assistant_users(id) ON DELETE CASCADE,
    version INTEGER NOT NULL CHECK (version > 0),
    note TEXT NOT NULL DEFAULT '',
    hard_criteria JSONB NOT NULL DEFAULT '{}'::jsonb,
    soft_criteria JSONB NOT NULL DEFAULT '{}'::jsonb,
    weights JSONB NOT NULL DEFAULT '{}'::jsonb,
    active_from TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, version)
);
CREATE TABLE vacancy_match_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES assistant_users(id) ON DELETE CASCADE,
    preference_id UUID NOT NULL REFERENCES vacancy_preferences(id) ON DELETE CASCADE,
    vacancy_id UUID NOT NULL REFERENCES vacancies(id) ON DELETE CASCADE,
    decision TEXT NOT NULL CHECK (decision IN ('match', 'reject', 'review')),
    method TEXT NOT NULL CHECK (method IN ('deterministic', 'ai')),
    score NUMERIC(5,4) NOT NULL CHECK (score >= 0 AND score <= 1),
    confidence TEXT CHECK (confidence IS NULL OR confidence IN ('low', 'medium', 'high')),
    rationale TEXT NOT NULL DEFAULT '',
    evidence_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    conflicts JSONB NOT NULL DEFAULT '[]'::jsonb,
    unknowns JSONB NOT NULL DEFAULT '[]'::jsonb,
    provider TEXT,
    model TEXT,
    prompt_version TEXT,
    input_snapshot_hash BYTEA,
    request_id TEXT,
    usage JSONB,
    status TEXT NOT NULL DEFAULT 'complete' CHECK (status IN ('complete', 'failed')),
    error_code TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, preference_id, vacancy_id, method, provider, model, prompt_version)
);
CREATE INDEX vacancy_match_results_user_created
    ON vacancy_match_results (user_id, created_at DESC);

CREATE TABLE telegram_connections (
    user_id UUID PRIMARY KEY REFERENCES assistant_users(id) ON DELETE CASCADE,
    chat_id BIGINT UNIQUE,
    linked_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    opted_in BOOLEAN NOT NULL DEFAULT false,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE telegram_deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES assistant_users(id) ON DELETE CASCADE,
    preference_id UUID NOT NULL REFERENCES vacancy_preferences(id) ON DELETE CASCADE,
    vacancy_id UUID NOT NULL REFERENCES vacancies(id) ON DELETE CASCADE,
    channel TEXT NOT NULL DEFAULT 'telegram' CHECK (channel = 'telegram'),
    status TEXT NOT NULL CHECK (status IN ('pending', 'sent', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_attempt_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at TIMESTAMPTZ,
    UNIQUE (user_id, preference_id, vacancy_id, channel)
);
