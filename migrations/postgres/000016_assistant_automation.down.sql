DROP INDEX IF EXISTS telegram_deliveries_claimable;
DROP INDEX IF EXISTS telegram_deliveries_idempotency;
ALTER TABLE telegram_deliveries
    DROP COLUMN IF EXISTS cooldown_until,
    DROP COLUMN IF EXISTS claimed_at,
    DROP COLUMN IF EXISTS notification_type,
    DROP COLUMN IF EXISTS provider_message_id;
DROP INDEX IF EXISTS assistant_work_items_lease;
DROP INDEX IF EXISTS assistant_work_items_claimable;
ALTER TABLE assistant_work_items
    DROP COLUMN IF EXISTS dead_letter_at,
    DROP COLUMN IF EXISTS completed_at,
    DROP COLUMN IF EXISTS claimed_by,
    DROP COLUMN IF EXISTS lease_until;
DROP TABLE IF EXISTS assistant_automation_settings;
