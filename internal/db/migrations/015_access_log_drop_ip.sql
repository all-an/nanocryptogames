-- 015_access_log_drop_ip.sql
-- Remove the ip column from access_log.
ALTER TABLE access_log DROP COLUMN IF EXISTS ip;
