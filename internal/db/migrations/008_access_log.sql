-- 008_access_log.sql
-- Tracks every page access with IP, country, and path for analytics.
-- access_daily aggregates the total hits per calendar day.
CREATE TABLE IF NOT EXISTS access_log (
    id          BIGSERIAL   PRIMARY KEY,
    ip          TEXT        NOT NULL,
    country     TEXT,
    path        TEXT        NOT NULL,
    accessed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS access_daily (
    day     DATE    PRIMARY KEY,
    count   BIGINT  NOT NULL DEFAULT 0
);
