-- 001_init.sql — initial schema for nano-multiplayer.
-- All CREATE statements use IF NOT EXISTS so the migration is safe to re-run.

-- Sequence used to assign deterministic HD wallet indices to players.
CREATE SEQUENCE IF NOT EXISTS player_seed_index_seq;

CREATE TABLE IF NOT EXISTS players (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    nano_address TEXT        NOT NULL UNIQUE,
    seed_index   INT         NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS game_sessions (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id           TEXT        NOT NULL,
    player_id         UUID        REFERENCES players(id),
    nano_deposited    NUMERIC     NOT NULL DEFAULT 0,
    nano_result       NUMERIC,                          -- NULL until settled
    shots_fired       INT         NOT NULL DEFAULT 0,
    balance_remaining NUMERIC     NOT NULL DEFAULT 0,
    settled_at        TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS nano_transactions (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID        REFERENCES game_sessions(id),
    direction  TEXT        NOT NULL CHECK (direction IN (
                               'deposit', 'withdrawal', 'shot', 'donation', 'heal_reward'
                           )),
    amount     NUMERIC     NOT NULL,
    block_hash TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Settings edited via admin panel without redeploying.
CREATE TABLE IF NOT EXISTS settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL
);

INSERT INTO settings (key, value) VALUES
    ('shot_cost',        '0.0001'),
    ('donation_amount',  '0.001'),
    ('donation_address', ''),
    ('heal_reward',      '0.0005')
ON CONFLICT (key) DO NOTHING;
