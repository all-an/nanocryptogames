-- 009_faucet.sql
-- Audit table for every faucet reward payout sent to players.
CREATE TABLE IF NOT EXISTS faucet_payouts (
    id          BIGSERIAL   PRIMARY KEY,
    reason      TEXT        NOT NULL,  -- "kill" or "heal"
    to_address  TEXT        NOT NULL,
    ip          TEXT,                  -- player's client IP (for anti-abuse checks)
    amount      TEXT        NOT NULL,  -- raw Nano units
    block_hash  TEXT,
    paid_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
