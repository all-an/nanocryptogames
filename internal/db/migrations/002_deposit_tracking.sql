-- 002_deposit_tracking.sql
-- Adds sender address tracking to nano_transactions so every incoming deposit
-- records where the Nano came from. This allows refunds if anything goes wrong.
ALTER TABLE nano_transactions ADD COLUMN IF NOT EXISTS from_address TEXT;
