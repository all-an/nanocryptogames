-- 013_rpg_color.sql: add player-chosen display color to rpg_accounts.
-- Empty string means "not yet chosen" — the server falls back to the palette.

ALTER TABLE rpg_accounts ADD COLUMN IF NOT EXISTS color TEXT NOT NULL DEFAULT '';
