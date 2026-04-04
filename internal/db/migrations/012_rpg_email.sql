-- 012_rpg_email.sql: add optional email column to rpg_accounts.
-- NULL is allowed so existing accounts are unaffected.
-- The partial unique index enforces uniqueness only among non-NULL values.

ALTER TABLE rpg_accounts ADD COLUMN IF NOT EXISTS email TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS rpg_accounts_email_unique
    ON rpg_accounts (email)
    WHERE email IS NOT NULL;
