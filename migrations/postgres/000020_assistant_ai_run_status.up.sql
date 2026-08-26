ALTER TABLE assistant_runs
    ADD COLUMN ai_status TEXT NOT NULL DEFAULT 'not_run'
        CHECK (ai_status IN ('not_run', 'pending', 'running', 'completed', 'partial', 'failed', 'skipped')),
    ADD COLUMN ai_skip_reason TEXT
        CHECK (ai_skip_reason IN (
            'server_disabled', 'user_opt_out', 'run_predates_ai', 'no_eligible',
            'budget_exhausted', 'already_analyzed', 'provider_unavailable', 'unknown'
        )),
    ADD COLUMN ai_eligible INTEGER NOT NULL DEFAULT 0 CHECK (ai_eligible >= 0),
    ADD COLUMN ai_succeeded INTEGER NOT NULL DEFAULT 0 CHECK (ai_succeeded >= 0);

-- Runs created before description-aware AI was deployed cannot be described by
-- the legacy zero counters. Mark them explicitly without inventing a runtime reason.
UPDATE assistant_runs
SET ai_status = 'skipped', ai_skip_reason = 'run_predates_ai'
WHERE created_at < TIMESTAMPTZ '2026-08-26 23:40:35+00'
  AND ai_calls = 0;

COMMENT ON COLUMN assistant_runs.ai_status IS
    'Provider-analysis lifecycle, independent from the deterministic run state.';
COMMENT ON COLUMN assistant_runs.ai_skip_reason IS
    'Stable reason why no provider analysis ran; NULL when AI ran or is pending.';
