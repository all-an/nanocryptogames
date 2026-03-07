-- 003_session_log.sql
-- Records every WebSocket session start with basic metadata for analytics.
CREATE TABLE IF NOT EXISTS session_log (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    player_id   UUID        REFERENCES players(id),
    nano_address TEXT,
    room_id     TEXT        NOT NULL,
    team        TEXT,
    remote_addr TEXT,
    started_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
