-- 007_rpc_test.sql
-- Adds the rpc_test setting (set to 'true' to enable the /rpc-test diagnostic page).
INSERT INTO settings (key, value)
VALUES ('rpc_test', 'false')
ON CONFLICT (key) DO NOTHING;
