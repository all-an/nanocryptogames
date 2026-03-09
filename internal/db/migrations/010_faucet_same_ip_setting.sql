-- 010_faucet_same_ip_setting.sql
-- Adds a toggle to disable the same-IP kill/heal reward block in faucet mode.
-- Set faucet_disable_same_ip_check = 'true' to allow same-IP rewards
-- (useful when players share a NAT or for local testing).
INSERT INTO settings (key, value) VALUES
    ('faucet_disable_same_ip_check', 'false')
ON CONFLICT (key) DO NOTHING;
