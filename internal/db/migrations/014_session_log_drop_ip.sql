-- 014_session_log_drop_ip.sql
-- Remove the remote_addr (IP) column from session_log.
ALTER TABLE session_log DROP COLUMN IF EXISTS remote_addr;
