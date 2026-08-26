DROP INDEX IF EXISTS telegram_link_tokens_expiry;
DROP TABLE IF EXISTS telegram_link_tokens;
DROP INDEX IF EXISTS telegram_deliveries_lease;
ALTER TABLE telegram_deliveries
    DROP COLUMN IF EXISTS dead_letter_at,
    DROP COLUMN IF EXISTS claimed_by,
    DROP COLUMN IF EXISTS lease_until;
