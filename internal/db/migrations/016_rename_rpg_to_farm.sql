-- 016_rename_rpg_to_farm.sql
-- Rename RPG tables to farm_ prefix for the Nano Farm game.
-- Uses conditional blocks so the migration is idempotent if tables were
-- already renamed manually (e.g. farm_sessions exists but rpg_sessions does not).
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'rpg_sessions')
     AND NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'farm_sessions') THEN
    ALTER TABLE rpg_sessions RENAME TO farm_sessions;
  END IF;

  IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'rpg_accounts')
     AND NOT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'farm_accounts') THEN
    ALTER TABLE rpg_accounts RENAME TO farm_accounts;
  END IF;
END $$;
