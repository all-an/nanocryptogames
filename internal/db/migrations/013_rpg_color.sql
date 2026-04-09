-- 013_farm_color.sql: add player-chosen display color to farm_accounts.
-- Empty string means "not yet chosen" — the server falls back to the palette.

ALTER TABLE farm_accounts ADD COLUMN IF NOT EXISTS color TEXT NOT NULL DEFAULT '';
