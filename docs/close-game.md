# How to Close the Game Temporarily

## Close the game

Run this SQL against your database:

```sql
UPDATE settings SET value = 'true' WHERE key = 'game_closed';
```

The change takes effect on the **next incoming request** — no server restart needed.
All visitors will see a lock screen instead of the game.

## Customise the message shown to players

```sql
UPDATE settings
SET value = 'We are back soon! Maintenance in progress.'
WHERE key = 'game_closed_message';
```

## Reopen the game

```sql
UPDATE settings SET value = 'false' WHERE key = 'game_closed';
```

## Check the current state

```sql
SELECT key, value FROM settings
WHERE key IN ('game_closed', 'game_closed_message');
```

## Notes

- Static assets (`/static/`) are always served so the closed page renders correctly.
- WebSocket connections and all game/lobby routes are blocked while closed.
- Player session wallets are unaffected — all funds remain accessible via the master seed.
- If `DATABASE_URL` is not set the middleware is skipped and the game is always open.
