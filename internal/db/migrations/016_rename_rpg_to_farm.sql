-- 016_rename_rpg_to_farm.sql
-- Rename RPG tables to farm_ prefix for the Nano Farm game.
ALTER TABLE IF EXISTS rpg_sessions  RENAME TO farm_sessions;
ALTER TABLE IF EXISTS rpg_accounts  RENAME TO farm_accounts;
