-- Durable assistant execution state. Public vacancy facts remain in vacancies.
CREATE TABLE assistant_work_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    source TEXT NOT NULL,
    external_id TEXT NOT NULL,
    available_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0 AND attempts <= 20),
    status TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'done', 'failed')),
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source, external_id)
);
CREATE INDEX assistant_work_items_pending
    ON assistant_work_items (available_at, id)
    WHERE status IN ('pending', 'failed');

CREATE TABLE assistant_cursors (
    source TEXT PRIMARY KEY,
    observed_at TIMESTAMPTZ NOT NULL DEFAULT 'epoch',
    external_id TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE assistant_ai_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES assistant_users(id) ON DELETE CASCADE,
    preference_id UUID NOT NULL REFERENCES vacancy_preferences(id) ON DELETE CASCADE,
    vacancy_id UUID NOT NULL REFERENCES vacancies(id) ON DELETE CASCADE,
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'complete', 'failed', 'disabled')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0 AND attempts <= 5),
    provider TEXT,
    model TEXT,
    request_id TEXT,
    input_snapshot_hash BYTEA,
    usage JSONB,
    error_code TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    UNIQUE (user_id, preference_id, vacancy_id)
);

ALTER TABLE vacancy_preferences
    ADD CONSTRAINT vacancy_preferences_note_length CHECK (char_length(note) <= 2000),
    ADD CONSTRAINT vacancy_preferences_hard_criteria_object CHECK (jsonb_typeof(hard_criteria) = 'object'),
    ADD CONSTRAINT vacancy_preferences_soft_criteria_object CHECK (jsonb_typeof(soft_criteria) = 'object'),
    ADD CONSTRAINT vacancy_preferences_weights_object CHECK (jsonb_typeof(weights) = 'object');

CREATE INDEX vacancy_match_results_preference_created
    ON vacancy_match_results (preference_id, created_at DESC);
CREATE INDEX telegram_deliveries_retryable
    ON telegram_deliveries (next_attempt_at, created_at)
    WHERE status IN ('pending', 'failed');

-- Request keys are retained separately so preference versions stay immutable
-- while retries return the original version instead of appending a duplicate.
CREATE TABLE assistant_preference_requests (
    user_id UUID NOT NULL REFERENCES assistant_users(id) ON DELETE CASCADE,
    request_id TEXT NOT NULL CHECK (char_length(request_id) BETWEEN 1 AND 255),
    preference_id UUID NOT NULL REFERENCES vacancy_preferences(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, request_id)
);

CREATE UNIQUE INDEX vacancy_match_results_dedup
    ON vacancy_match_results (
        user_id, preference_id, vacancy_id, method,
        COALESCE(provider, ''), COALESCE(model, ''), COALESCE(prompt_version, '')
    );
