# CLAUDE.md

## Code Style

- Small functions — one responsibility, fits on screen
- Small structs — group only what belongs together
- Comment every exported function, type, and non-obvious block
- Follow standard Go conventions (gofmt, golint)

## Testing

- Write unit tests for every function with logic
- Table-driven tests preferred
- Use interfaces and mocks to isolate dependencies (especially NanoRPC and DB)

## General

- Prefer clarity over cleverness
- No premature abstractions — solve the problem in front of you
- Keep dependencies minimal

## Project Structure

- `internal/games/` — shared faucet game logic (package `games`)
- `internal/games/rpg/` — Nano Faucet Multiplayer RPG game logic (package `rpg`)
- `internal/handler/` — HTTP handlers for all games
- `internal/db/` — PostgreSQL layer; migrations in `internal/db/migrations/`
- `internal/nano/` — Nano RPC, wallet derivation, block signing
- `internal/templates/faucet_game/` — HTML templates for the faucet shooter game
- `internal/templates/faucet_multiplayer_rpg_templates/` — HTML templates for the RPG
- `web/static/` — shared static assets; `web/static/faucet_multiplayer_rpg_static/` for RPG assets

## Skills

Custom AI persona prompts live in `.claude/skills/<name>/SKILL.md`:

- `/storyteller` — expert narrative craft, world-building, RPG dialogue
- `/gamedev` — expert game developer (math, physics, multiplayer, RPG systems)
- `/webdev` — expert web developer (HTML/CSS/JS, Go net/http, security, performance)
