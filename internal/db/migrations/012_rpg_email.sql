-- 012_farm_email.sql: add optional email column to farm_accounts.
-- NULL is allowed so existing accounts are unaffected.
-- The partial unique index enforces uniqueness only among non-NULL values.

ALTER TABLE farm_accounts ADD COLUMN IF NOT EXISTS email TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS farm_accounts_email_unique
    ON farm_accounts (email)
    WHERE email IS NOT NULL;
