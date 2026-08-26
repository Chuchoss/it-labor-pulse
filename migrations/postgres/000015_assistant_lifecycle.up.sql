ALTER TABLE vacancy_preferences ADD COLUMN archived_at TIMESTAMPTZ;
CREATE INDEX vacancy_preferences_active ON vacancy_preferences (user_id, version DESC)
    WHERE archived_at IS NULL;

CREATE TABLE assistant_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES assistant_users(id) ON DELETE CASCADE,
    state TEXT NOT NULL CHECK (state IN ('queued', 'running', 'succeeded', 'failed', 'disabled')),
    request_id TEXT,
    started_at TIMESTAMPTZ,
    finished_at TIMESTAMPTZ,
    last_checked_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    processed INTEGER NOT NULL DEFAULT 0 CHECK (processed >= 0),
    eligible INTEGER NOT NULL DEFAULT 0 CHECK (eligible >= 0),
    matched INTEGER NOT NULL DEFAULT 0 CHECK (matched >= 0),
    ai_calls INTEGER NOT NULL DEFAULT 0 CHECK (ai_calls >= 0),
    skipped INTEGER NOT NULL DEFAULT 0 CHECK (skipped >= 0),
    error_category TEXT,
    cursor_source TEXT NOT NULL DEFAULT 'hh',
    cursor_observed_at TIMESTAMPTZ,
    pending_candidates BOOLEAN NOT NULL DEFAULT false,
    provider TEXT,
    model TEXT,
    prompt_version TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX assistant_runs_user_created ON assistant_runs (user_id, created_at DESC);
