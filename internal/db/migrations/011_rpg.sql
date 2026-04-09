-- 011_farm.sql: Nano Faucet Multiplayer Farm — accounts and sessions.

CREATE TABLE IF NOT EXISTS farm_accounts (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    username      TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    seed_index    INTEGER     NOT NULL,
    nano_address  TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS farm_sessions (
    token      TEXT        PRIMARY KEY,
    account_id UUID        NOT NULL REFERENCES farm_accounts(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen  TIMESTAMPTZ NOT NULL DEFAULT now()
);
