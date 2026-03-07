# Nano Multiplayer Game — Roadmap

## Project Overview

A real-money multiplayer browser game built on Go + WebSockets, using Nano (XNO) cryptocurrency for deposits and payouts. Server-authoritative game loop, SSR HTML, and zero Redis dependency.

---

## Stack

| Layer       | Technology                          |
|-------------|-------------------------------------|
| Server      | Go + net/http or Fiber/Echo         |
| Realtime    | Go WebSockets (gorilla/websocket)   |
| Rendering   | Go html/template (SSR)              |
| DB          | PostgreSQL on Render.com            |
| Nano        | HTTP calls to Nano RPC node         |
| Deploy      | Render.com Web Service              |

---

## Project Structure

```
cmd/
  server/
    main.go
internal/
  nano/
    wallet.go          # derive addresses from seed
    rpc.go             # calls to Nano RPC node
    transaction.go     # send/receive logic
  game/
    room.go            # room lifecycle, ticker loop
    player.go          # per-player state
    physics.go         # server-side movement validation
    hub.go             # central WS hub (replaces Redis pub/sub)
  db/
    postgres.go        # connection pool (pgx)
    migrations/
      001_init.sql
  handler/
    home.go
    game.go
    rooms.go
  templates/
    lobby.html
    game.html
web/
  static/
    game.js            # canvas + WS client
    style.css
```

---

## Phase 1 — Foundation (MVP) ✅ DONE

**Goal:** Running server, WebSocket hub, game loop, SSR pages.

- [x] Scaffold Go module (`go mod init`)
- [x] `cmd/server/main.go` — HTTP server + route wiring
- [x] `internal/game/hub.go` — central Hub goroutine (mutex-based, no Redis)
- [x] `internal/game/room.go` — room struct, 20 TPS ticker loop
- [x] `internal/game/player.go` — per-player state
- [x] `internal/game/physics.go` — grid constants, movement validation
- [x] `handler/home.go` + `handler/game.go` — serve SSR templates
- [x] `templates/` — lobby and game HTML
- [x] `web/static/game.js` — Canvas renderer + WebSocket client
- [x] Player avatar: filled circle with unique colour per player, Ӿ symbol centred inside
- [x] Grid-based movement (25×17 grid, 40×40px cells = player diameter)
- [x] 5-cell Euclidean radius movement — server-validated, reachable area shown on canvas
- [x] Smooth cell-by-cell path animation (Chebyshev waypoints, 6px/frame)
- [x] Lobby shows live active room list, auto-refreshes every 3 s
- [x] 36 unit tests — physics, player, room, nano wallet, RPC client

---

## Phase 2 — Database + Persistence ✅ DONE

**Goal:** Postgres wired up, migrations, player/session records.

- [x] `internal/db/postgres.go` — pgx connection pool
- [x] `migrations/001_init.sql` — full schema (embedded in binary, safe to re-run)
- [x] Player creation on first connect (derive wallet, store seed index)
- [x] `game_sessions` row created on room join
- [x] `settings` table seeded with shot_cost, donation_amount, heal_reward
- [x] DB is optional — server runs without `DATABASE_URL` (local dev)

---

## Phase 3 — Nano HD Wallet + RPC Client ✅ DONE

**Goal:** Derive session wallets per player; talk to public Nano nodes.

- [x] `internal/nano/wallet.go` — Blake2b-256 + ed25519 HD key derivation, nano_ address encoding/decoding
- [x] `internal/nano/rpc.go` — HTTP client with primary (NanOslo) + automatic fallback (SomeNano)
- [x] `internal/nano/transaction.go` — full state block creation, signing, Send + ReceivePending
- [x] On WS connect: reserve seed index from DB → derive nano_ address → persist player + session
- [x] `NANO_MASTER_SEED` from env; ephemeral random seed with warning when unset

### Public Node Options

| Priority  | Node     | URL                       | Version | Tokens | WebSocket |
|-----------|----------|---------------------------|---------|--------|-----------|
| Primary   | NanOslo  | https://nanoslo.0x.no     | V28.2   | 10,000 | Full      |
| Secondary | SomeNano | https://node.somenano.com | V28.2   | 5,000  | Full      |

---

## ▶ Phase 4 — Combat System  ← START HERE

**Goal:** Server-authoritative shooting. One hit incapacitates; two hits kill.

### Combat States

| State | Health | Can move? | Can shoot? | Appearance |
|---|---|---|---|---|
| Healthy | 100 | Yes | Yes | Full colour |
| Incapacitated | 50 | No | No | Dimmed, Ӿ replaced with ✕ |
| Dead | 0 | No | No | Greyed out, removed after 2 s |

### Shooting UX

When a player clicks on a cell occupied by an **enemy**, the modal shows:

```
┌─────────────────────┐
│  What do you want?  │
│  [Move]  [Shoot]    │
└─────────────────────┘
```

Clicking **Shoot** fires an animated bullet (fast dot, ~200 ms) from shooter to target; hit applied after animation completes.

- [ ] Modal detects enemy in clicked cell → shows Move + Shoot buttons
- [ ] `{"action":"shoot","targetID":"..."}` sent to server on Shoot click
- [ ] Client plays bullet animation (dot travels cell-by-cell path, 200 ms total)
- [ ] Server validates: shooter alive, not incapacitated, target in range
- [ ] First hit → health 50, incapacitated (cannot move or shoot)
- [ ] Second hit → health 0, dead; removed from room after 2 s grace period
- [ ] Dead player can still withdraw remaining Nano balance
- [ ] Broadcast updated health state immediately after each hit
- [ ] Canvas: incapacitated = dimmed circle + ✕ glyph; dead = greyed out

---

## Phase 5 — Medkit System

**Goal:** Players can heal incapacitated teammates. Healing earns a Nano fraction.

- [ ] Medkit items spawn at fixed grid positions each round
- [ ] `{"action":"pickup"}` — player on a medkit cell picks it up (max 1 held)
- [ ] `{"action":"heal","targetID":"..."}` — uses held medkit on adjacent incapacitated player
- [ ] Healed player returns to full health (100)
- [ ] Server credits healer's `balance_remaining` with `heal_reward` from settings
- [ ] Record in `nano_transactions` with `direction = 'heal_reward'`
- [ ] Medkit shown on canvas as green cross `✚` in the grid cell

---

## Phase 5b — Team Communication

**Goal:** Players can send short messages to teammates. Messages appear as fading popups.

- [ ] Click on a teammate's circle → small text input overlay appears (max 80 chars)
- [ ] Press Enter → `{"action":"msg","targetID":"...","text":"..."}` sent to server
- [ ] Server validates, strips control characters, relays only to target player's WS
- [ ] Client renders message bubble above circle; fades out after 4 s
- [ ] Not persisted — in-memory relay only

---

## Phase 7 — Deployment on Render.com

**Goal:** Live on Render with free Postgres, secret env vars.

- [ ] `render.yaml` at project root
- [ ] Set `NANO_MASTER_SEED` as secret env var in Render dashboard (never in code)
- [ ] Confirm `DATABASE_URL` auto-injected from linked Postgres instance
- [ ] Verify sleep behaviour on free tier; upgrade to $7/mo Starter if needed

### render.yaml

```yaml
services:
  - type: web
    name: nano-shooter
    runtime: go
    buildCommand: go build -o bin/server ./cmd/server
    startCommand: ./bin/server
    envVars:
      - key: DATABASE_URL
        fromDatabase:
          name: nano-shooter-db
          property: connectionString
      - key: NANO_RPC_PRIMARY_URL
        value: https://nanoslo.0x.no
      - key: NANO_RPC_FALLBACK_URL
        value: https://node.somenano.com
      - key: NANO_MASTER_SEED
        sync: false   # set manually in dashboard

databases:
  - name: nano-shooter-db
    databaseName: nano_shooter
    plan: free
```

---

## Phase 8 — Self-Hosted Nano Node

**Goal:** Replace public node with own node for reliability, push notifications, and privacy.

**Trigger:** Real players + real money, or hitting public node rate limits.

- [ ] Provision VPS (Hetzner $12/mo or Contabo $6/mo — not Render, needs persistent disk)
  - RAM: 2–4 GB, CPU: 2 cores, Disk: ~100 GB SSD
- [ ] Sync Nano ledger (~100 GB)
- [ ] Switch `NanoRPC` implementation to own node URL (env var change only)
- [ ] Migrate polling → WebSocket block push notifications

---

## Phase 9 — Shot Economy & Donation

**Goal:** Every shot costs a micro-amount of Nano. The first shot fires an async donation to the owner.

| Event | Nano amount | Destination |
|---|---|---|
| First shot fired (per session) | `donation_amount` from settings | owner address — sent async, no game delay |
| Each subsequent shot | `shot_cost` from settings | deducted from player session balance |
| Match winner payout | accumulated shot fees | winner's withdraw address |

- [ ] Track `shots_fired` and `balance_remaining` per `game_sessions` row
- [ ] On first shot: goroutine sends `donation_amount` → owner; game loop not blocked
- [ ] On each shot: deduct `shot_cost` from in-memory balance; settle to Postgres on round end
- [ ] Server rejects shoot inputs from players with `balance_remaining <= 0`

---

## Phase 10 — Player Withdrawal

**Goal:** After a match, players withdraw remaining Nano to any address — no account required.

- [ ] After game ends, show withdrawal form on game page
- [ ] Player enters their Nano address → server validates → calls `nano.Send`
- [ ] Record in `nano_transactions` with `direction = 'withdrawal'`
- [ ] Show confirmed block hash as receipt
- [ ] Unclaimed balances expire after 24 h (cleanup job in Phase 12)

---

## Phase 11 — Admin Panel

**Goal:** Owner-only UI to view stats and adjust economy settings live.

- [ ] `GET  /admin` → login form (password from `ADMIN_SECRET` env var)
- [ ] `POST /admin/login` → signed session cookie (`crypto/hmac`)
- [ ] `GET  /admin/dashboard` → stats (sessions, Nano donated, Nano paid out, active rooms)
- [ ] `POST /admin/settings` → update `settings` table live (shot_cost, donation_amount, donation_address)
- [ ] `GET  /admin/logout` → clear cookie

---

## Phase 12 — Hardening & Scale

- [ ] End-to-end tests for deposit → shoot → payout → withdrawal flow
- [ ] Cleanup job: sweep unclaimed session balances after 24 h
- [ ] Rate limiting on WebSocket connections and RPC calls
- [ ] Graceful shutdown: drain rooms, settle open sessions
- [ ] Metrics (Prometheus endpoint or Render built-in)
- [ ] Consider Redis only if horizontal scaling (multiple Render instances) becomes necessary

---

## Key Security Rules

1. **NANO_MASTER_SEED** — secret env var only, never in source or logs.
2. **Wallet derivation** — `deriveKeyPair(masterSeed, playerIndex)`; index stored in Postgres.
3. **Server-authoritative** — all movement, combat, shot deduction, and balance checks run server-side.
4. **SSR + WebSocket** — same Go HTTP server handles both; upgrade `/ws/{roomID}` to WS.
5. **Admin auth** — session cookie signed with `ADMIN_SECRET` env var; constant-time comparison.
6. **Donation address** — stored in `settings` table, editable only via authenticated admin panel.

---

## Decision Log

| Decision | Rationale |
|---|---|
| No Redis | Go channels handle in-memory pub/sub; Redis only needed for horizontal scale |
| Public node first | Low RPC volume at MVP scale; own node when hitting rate limits or paying real money |
| NanoRPC interface from day 1 | Lets you swap node implementation without touching game logic |
| Render free tier | Zero infra cost for MVP; upgrade to $7/mo Starter to prevent sleep |
| pgx over database/sql | Better Postgres performance, native types, connection pool |
| No player login | Nano address = identity; session wallet is the account; no friction to play |
| First shot = async donation | Donation fires in background goroutine — game loop is never blocked |
| Settings in DB not env vars | Owner can change shot cost and donation address live without redeploying |
| Single-password admin | No user management complexity; one `ADMIN_SECRET` env var guards the panel |
| Chebyshev path animation | Movement steps through cell centres — no straight-line pixel teleport |
