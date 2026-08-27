ALTER TABLE assistant_runs
    ADD COLUMN superseded_from_state TEXT
        CHECK (superseded_from_state IN ('queued', 'running', 'paused', 'succeeded', 'failed'));

COMMENT ON COLUMN assistant_runs.superseded_from_state IS
    'State immediately before the snapshot became obsolete; preserves whether it had already finished.';
