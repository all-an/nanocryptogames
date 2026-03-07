-- 005_game_closed.sql
-- Adds a toggle to close the game temporarily.
-- Set game_closed = 'true' to show a maintenance screen to all visitors.
-- Set game_closed_message to customise the message shown.
INSERT INTO settings (key, value) VALUES
    ('game_closed',         'false'),
    ('game_closed_message', 'The game is temporarily closed. Check back soon!')
ON CONFLICT (key) DO NOTHING;
