-- Durable per-user automation controls. All switches are explicit opt-in.
CREATE TABLE assistant_automation_settings (
    user_id UUID PRIMARY KEY REFERENCES assistant_users(id) ON DELETE CASCADE,
    ai_enabled BOOLEAN NOT NULL DEFAULT false,
    telegram_enabled BOOLEAN NOT NULL DEFAULT false,
    activation_at TIMESTAMPTZ,
    max_ai_calls_per_hour INTEGER NOT NULL DEFAULT 20
        CHECK (max_ai_calls_per_hour BETWEEN 1 AND 100),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (NOT ai_enabled OR activation_at IS NOT NULL)
);

ALTER TABLE assistant_work_items
    ADD COLUMN lease_until TIMESTAMPTZ,
    ADD COLUMN claimed_by TEXT,
    ADD COLUMN completed_at TIMESTAMPTZ,
    ADD COLUMN dead_letter_at TIMESTAMPTZ;

CREATE INDEX assistant_work_items_claimable
    ON assistant_work_items (available_at, id)
    WHERE status IN ('pending', 'failed');

CREATE INDEX assistant_work_items_lease
    ON assistant_work_items (lease_until)
    WHERE status = 'processing';

ALTER TABLE telegram_deliveries
    ADD COLUMN provider_message_id TEXT,
    ADD COLUMN notification_type TEXT NOT NULL DEFAULT 'match',
    ADD COLUMN claimed_at TIMESTAMPTZ,
    ADD COLUMN cooldown_until TIMESTAMPTZ;

CREATE UNIQUE INDEX telegram_deliveries_idempotency
    ON telegram_deliveries (user_id, preference_id, vacancy_id, notification_type);

CREATE INDEX telegram_deliveries_claimable
    ON telegram_deliveries (next_attempt_at, created_at)
    WHERE status IN ('pending', 'failed');
