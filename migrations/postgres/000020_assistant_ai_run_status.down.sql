ALTER TABLE assistant_runs
    DROP COLUMN ai_succeeded,
    DROP COLUMN ai_eligible,
    DROP COLUMN ai_skip_reason,
    DROP COLUMN ai_status;
