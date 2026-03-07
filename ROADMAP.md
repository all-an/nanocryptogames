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
    physics.go         # server-side hit detection
    hub.go             # central WS hub (replaces Redis pub/sub)
  db/
    postgres.go        # connection pool (pgx)
    migrations/
      001_init.sql
  handler/
    home.go
    game.go
    deposit.go
  templates/
    base.html
    lobby.html
    game.html
web/
  static/
    game.js            # canvas + WS client
    style.css
```

---

## Phase 1 — Foundation (MVP)

**Goal:** Running server, WebSocket hub, game loop, SSR pages.

- [ ] Scaffold Go module (`go mod init`)
- [ ] `cmd/server/main.go` — HTTP server + route wiring
- [ ] `internal/game/hub.go` — central Hub goroutine (channels, no Redis)
- [ ] `internal/game/room.go` — room struct, 20 TPS ticker loop
- [ ] `internal/game/player.go` — per-player state
- [ ] `internal/game/physics.go` — server-side collision/hit detection
- [ ] `handler/home.go` + `handler/game.go` — serve SSR templates
- [ ] `templates/` — base, lobby, and game HTML
- [ ] `web/static/game.js` — Canvas renderer + WebSocket client
- [ ] Player avatar: filled circle with unique colour per player, Ӿ symbol centred inside
- [ ] Grid-based movement (25×17 grid, 40×40px cells = player diameter)
- [ ] Lobby shows live active room list, auto-refreshes every 3 s
- [ ] Local smoke test: two browsers, one room, no money

### Player Avatar Design

Each player renders as a solid circle on the Canvas. The Ӿ glyph is drawn centred inside using `fillText`, making the Nano identity immediately visible in-game.

```
  ┌──────────┐
  │  ╔════╗  │
  │  ║ Ӿ  ║  │  ← unique colour per player (assigned on join)
  │  ╚════╝  │
  └──────────┘
```

Colour is assigned server-side from a fixed palette and sent as part of the player state, so all clients agree on who is which colour:

```js
// game.js — draw one player
function drawPlayer(ctx, player) {
  ctx.beginPath();
  ctx.arc(player.x, player.y, 20, 0, Math.PI * 2);
  ctx.fillStyle = player.color;
  ctx.fill();

  ctx.fillStyle = "#fff";
  ctx.font = "bold 16px sans-serif";
  ctx.textAlign = "center";
  ctx.textBaseline = "middle";
  ctx.fillText("Ӿ", player.x, player.y);
}
```

Palette (server assigns index on join, wraps if > 8 players):

| Index | Colour    | Hex       |
|-------|-----------|-----------|
| 0     | Nano blue | `#4A90D9` |
| 1     | Crimson   | `#E05252` |
| 2     | Emerald   | `#52C07A` |
| 3     | Amber     | `#F5A623` |
| 4     | Violet    | `#9B59B6` |
| 5     | Teal      | `#1ABC9C` |
| 6     | Orange    | `#E67E22` |
| 7     | Rose      | `#E91E8C` |

### Hub Design (replaces Redis pub/sub)

```go
type Hub struct {
    rooms      map[string]*Room
    register   chan *Player
    unregister chan *Player
    mu         sync.RWMutex
}
```

### Game Loop (goroutine per room)

```go
func (r *Room) runTick() {
    ticker := time.NewTicker(50 * time.Millisecond) // 20 TPS
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            r.processInputs()
            r.checkCollisions()
            r.broadcastState()
        case input := <-r.inputCh:
            r.applyInput(input)
        case <-r.done:
            r.settle()
            return
        }
    }
}
```

---

## Phase 2 — Database + Persistence

**Goal:** Postgres wired up, migrations, player/session records.

- [ ] `internal/db/postgres.go` — pgx connection pool
- [ ] `migrations/001_init.sql` — schema below
- [ ] Player creation on first connect (derive wallet, store index)
- [ ] `game_sessions` row created on room join
- [ ] Session settlement writes to `nano_transactions` on room end

### Schema

```sql
CREATE TABLE players (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    nano_address TEXT NOT NULL UNIQUE,
    seed_index   INT NOT NULL,
    created_at   TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE game_sessions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id         TEXT NOT NULL,
    player_id       UUID REFERENCES players(id),
    nano_deposited  NUMERIC NOT NULL DEFAULT 0,
    nano_result     NUMERIC,
    settled_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE nano_transactions (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id  UUID REFERENCES game_sessions(id),
    direction   TEXT CHECK (direction IN ('deposit', 'withdrawal')),
    amount      NUMERIC NOT NULL,
    block_hash  TEXT,
    created_at  TIMESTAMPTZ DEFAULT now()
);
```

---

## Phase 3 — Nano Integration (Public Node)

**Goal:** Deposits working end-to-end with a public Nano RPC node.

- [ ] `internal/nano/wallet.go` — HD key derivation from master seed + player index
- [ ] `internal/nano/rpc.go` — implement `NanoRPC` interface against public node
- [ ] `internal/nano/transaction.go` — send/receive/pending logic
- [ ] `handler/deposit.go` — deposit flow, show player address QR
- [ ] Polling loop: `time.Ticker` every 2s per active deposit address
- [ ] Credit player on confirmed block, update `nano_transactions`

### NanoRPC Interface (swap-ready)

```go
type NanoRPC interface {
    GetBalance(address string) (decimal.Decimal, error)
    Send(from, to string, amount decimal.Decimal) (string, error)
    ReceivePending(address string) error
}
```

### Public Node Options

| Priority  | Node          | URL                          | Version  | Tokens | WebSocket | Notes                                         |
|-----------|---------------|------------------------------|----------|--------|-----------|-----------------------------------------------|
| Primary   | NanOslo       | https://nanoslo.0x.no        | V28.2    | 10,000 | Full      | Most generous token limit, all tests pass     |
| Secondary | SomeNano      | https://node.somenano.com    | V28.2    | 5,000  | Full      | Fastest response, latest version              |

Both nodes are on the latest Nano version with full WebSocket support. Wire the secondary as an automatic fallback:

```go
// rpc.go
type Config struct {
    PrimaryURL  string // NanOslo
    FallbackURL string // SomeNano
}
```

Source: [publicnodes.somenano.com](https://publicnodes.somenano.com/) — recheck before going to production as uptime rankings shift.

---

## Phase 4 — Combat System

**Goal:** Server-authoritative shooting. One hit incapacitates; two hits kill.

### Combat States

| State | Health | Can move? | Can shoot? | Appearance |
|---|---|---|---|---|
| Healthy | 100 | Yes | Yes | Full colour |
| Incapacitated | 50 | No | No | Dimmed, Ӿ replaced with ✕ |
| Dead | 0 | No | No | Greyed out, removed after 2 s |

### Shooting UX

When a player clicks on a cell occupied by an **enemy**, the modal shows two buttons:

```
┌─────────────────────┐
│  What do you want?  │
│  [Move]  [Shoot]    │
└─────────────────────┘
```

Clicking **Shoot** fires an animated bullet from the shooter's cell to the target cell. The bullet is a fast-moving dot rendered on the Canvas that completes in ~200 ms, then the hit is applied and health updated.

- [ ] Modal detects whether clicked cell contains an enemy or is empty; renders Move / Move+Shoot accordingly
- [ ] `{"action":"shoot","targetID":"..."}` sent to server on Shoot click
- [ ] Client plays a bullet animation (fast dot from src → dst over 200 ms) before showing the hit result
- [ ] Server validates and applies damage; broadcasts updated health immediately
- [ ] Add `shoot` action to Input: `{"action":"shoot","targetID":"..."}`
- [ ] Server validates shooter is alive and target is in range (adjacent or line-of-sight)
- [ ] First hit sets target health to 50 → incapacitated (cannot move or shoot)
- [ ] Second hit sets health to 0 → dead; player removed from room after 2 s grace period
- [ ] Dead player can still withdraw remaining Nano balance
- [ ] Broadcast updated health state immediately after a hit
- [ ] Incapacitated state shown on canvas: dimmed circle, Ӿ replaced with ✕ symbol

---

## Phase 5 — Medkit System

**Goal:** Players can heal incapacitated teammates. Healing earns the healer a Nano fraction.

### Medkit Rules

- A player can use a medkit on an **incapacitated** (not dead) teammate on an adjacent cell
- Healed player returns to full health (100) and can move and shoot again
- Healer receives a small Nano reward (e.g. 0.0005 Ӿ) credited to their session balance
- Medkits are consumable items found on the map (spawn at fixed grid positions each round)

### Implementation

- [ ] Add medkit spawn positions to room state; broadcast as part of `worldState`
- [ ] `{"action":"pickup"}` — player on a medkit cell picks it up; max 1 held per player
- [ ] `{"action":"heal","targetID":"..."}` — uses held medkit on adjacent incapacitated player
- [ ] Server credits healer's `balance_remaining` with the heal reward
- [ ] Record heal reward in `nano_transactions` with a `heal_reward` direction
- [ ] Medkit shown on canvas as a green cross `✚` in the grid cell

```sql
ALTER TABLE game_sessions ADD COLUMN heals_given INT NOT NULL DEFAULT 0;
-- heal_reward stored in settings table alongside shot_cost
-- INSERT INTO settings VALUES ('heal_reward', '0.0005');
```

---

## Phase 5b — Team Communication

**Goal:** Players can send short messages to teammates during a match. No persistent chat — messages appear as popups and fade.

### UX Flow

```
Player clicks on a teammate's circle
  → Small input overlay appears (max 80 chars)
  → Player types and presses Enter
  → Message delivered via WebSocket to that specific player only
  → Recipient sees a popup bubble above their circle for 4 s
```

### Implementation

- [ ] `{"action":"msg","targetID":"...","text":"..."}` — sent from client to server
- [ ] Server validates sender and target are in the same room; strips control characters
- [ ] Server relays `{"type":"msg","fromID":"...","text":"..."}` only to the target player's WebSocket
- [ ] Client renders message bubble above the target's circle using Canvas `fillText`; fades out after 4 s
- [ ] Messages are not persisted — in-memory relay only

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

```go
// With own node — push, no polling
conn.Subscribe("confirmation", func(block NanoBlock) {
    if isDepositAddress(block.Account) {
        creditPlayer(block.Account, block.Amount)
    }
})
```

---

## Phase 9 — Shot Economy & Donation

**Goal:** Every shot costs a micro-amount of Nano. The first shot fired during play triggers a one-time donation to the owner — the game continues normally, it happens in the background.

### Economy Design

| Event | Nano amount | Destination |
|---|---|---|
| First shot fired (per session) | fixed (e.g. 0.001 Ӿ) | owner donation address — sent async |
| Each subsequent shot | micro (e.g. 0.0001 Ӿ) | deducted from player's session balance |
| Match winner payout | accumulated shot fees in room | winner's withdraw address |

- The donation is **not a gate** — the shot fires immediately and the Nano transfer happens in a background goroutine without blocking the game loop.
- Players with zero balance can no longer fire — server enforces this; client is display only.
- No login required; balance is tied to the session wallet derived from `(master_seed, player_index)`.

### Implementation

- [ ] Add `settings` table for owner-configurable values (`shot_cost`, `donation_amount`, `donation_address`)
- [ ] Track `shots_fired` and `balance_remaining` per `game_sessions` row
- [ ] On first shot: spin up a goroutine to send `donation_amount` → owner address; game loop not blocked
- [ ] On each shot: deduct `shot_cost` from in-memory balance; settle to Postgres on round end
- [ ] Server rejects shoot inputs from players with `balance_remaining <= 0`

### Schema additions

```sql
CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
-- Seed rows:
-- INSERT INTO settings VALUES ('shot_cost',        '0.0001');
-- INSERT INTO settings VALUES ('donation_amount',  '0.001');
-- INSERT INTO settings VALUES ('donation_address', 'nano_1...');

ALTER TABLE game_sessions
    ADD COLUMN shots_fired       INT     NOT NULL DEFAULT 0,
    ADD COLUMN balance_remaining NUMERIC NOT NULL DEFAULT 0;
```

---

## Phase 10 — Player Withdrawal

**Goal:** After a match (or at any point), a player can withdraw their remaining Nano balance to any address — no account required.

- [ ] After game ends, show a withdrawal form on the game page
- [ ] Player enters their own Nano address and clicks Withdraw
- [ ] Server validates address format, calls `nano.Send(sessionWallet, playerAddress, balance)`
- [ ] Record withdrawal in `nano_transactions` with `direction = 'withdrawal'`
- [ ] Show block hash as confirmation
- [ ] Unclaimed balances expire after 24 h (cleanup job in Phase 9)

### UX flow

```
Match ends → winner banner shown
  → "Withdraw your Ӿ" input appears
  → Player pastes Nano address → clicks Withdraw
  → Server sends Nano, shows block hash
  → Done — no account, no cookie, no email needed
```

---

## Phase 11 — Admin Panel

**Goal:** Owner-only web UI to view stats, adjust shot economy, and update the donation address — without redeploying.

- [ ] `GET  /admin` → login form (password from `ADMIN_SECRET` env var)
- [ ] `POST /admin/login` → sets a signed session cookie (`crypto/hmac`)
- [ ] `GET  /admin/dashboard` → stats overview (session-guarded)
- [ ] `POST /admin/settings` → update `settings` table live
- [ ] `GET  /admin/logout` → clear cookie

### Stats shown on dashboard

| Stat | Source |
|---|---|
| Total sessions played | `COUNT(*) FROM game_sessions` |
| Total Nano donated | sum of first-shot donation transactions |
| Total Nano paid out | `SUM WHERE direction='withdrawal'` |
| Active rooms right now | in-memory from Hub |

### Settings panel fields

- **Donation amount** — Ӿ sent to owner on first shot per session (async, during play)
- **Shot cost** — Ӿ deducted per shot after the first
- **Donation address** — your Nano address to receive donations

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
3. **Server-authoritative** — all collision, shot deduction, and balance checks run server-side.
4. **SSR + WebSocket** — same Go HTTP server handles both; upgrade `/ws/{roomID}` to WS.
5. **Admin auth** — session cookie signed with `ADMIN_SECRET` env var; constant-time comparison against brute force.
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
