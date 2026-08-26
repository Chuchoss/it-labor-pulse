-- Durable Telegram delivery leases, dead-letter state, and one-time link challenges.
ALTER TABLE telegram_deliveries
    ADD COLUMN lease_until TIMESTAMPTZ,
    ADD COLUMN claimed_by TEXT,
    ADD COLUMN dead_letter_at TIMESTAMPTZ;

CREATE INDEX telegram_deliveries_lease
    ON telegram_deliveries (lease_until)
    WHERE status IN ('pending', 'failed');

CREATE TABLE telegram_link_tokens (
    token_hash BYTEA PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES assistant_users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX telegram_link_tokens_expiry ON telegram_link_tokens (expires_at);
