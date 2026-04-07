# Nano Simple Wallet — Roadmap

A lightweight, browser-based Nano wallet with no sign-up and no server-side key storage.
The seed is generated server-side via `crypto/rand`, shown once to the user, and never persisted.

---

## Milestone 1 — Wallet Generation (done)

- [x] `internal/wallet/generate.go` — `Generate()` creates random 32-byte seed, derives address at index 0
- [x] `internal/wallet/generate_test.go` — unit tests (seed length, hex format, address format, uniqueness)
- [x] `internal/handler/wallet.go` — `POST /wallet/create` returns `{seed, address}` JSON
- [x] `web/static/wallet/wallet.css` — shiny modal styles
- [x] `web/static/wallet/wallet.js` — fetch → modal with seed display and clipboard copy button
- [x] Landing page "Create Wallet" button wired to modal flow
- [x] Route registered in `cmd/server/main.go`

---

## Milestone 2 — View Balance

- [ ] Wallet page at `/wallet` — user pastes their seed or address
- [ ] `GET /wallet/balance?address=nano_...` — proxies `account_info` + `receivable` from Nano RPC
- [ ] Display confirmed balance and pending receivable in the UI
- [ ] No seed required to check balance (address is public)

---

## Milestone 3 — Receive (open / pocket pending blocks)

- [ ] `POST /wallet/receive` — accepts `{seed}`, calls `nano.ReceivePending` for index-0 wallet
- [ ] Show incoming transactions with sender address and amount
- [ ] QR code for the wallet address (reuse `api.qrserver.com` pattern)
- [ ] Warning: seed is sent over HTTPS and used only in-memory; never logged or stored

---

## Milestone 4 — Send

- [ ] `POST /wallet/send` — accepts `{seed, to, amountRaw}`
- [ ] Server-side validation: valid nano_ address, amount > 0, sufficient balance
- [ ] Return block hash on success; display in UI with link to Nano block explorer
- [ ] Rate-limit: 1 send per IP per 10 seconds to protect PoW resources
- [ ] PoW generated on demand (no pre-cache for user wallets)

---

## Milestone 5 — Transaction History

- [ ] `GET /wallet/history?address=nano_...` — proxies `account_history` from Nano RPC
- [ ] Display last 20 send/receive blocks with timestamps, amounts, and counterparty address
- [ ] Paginate with `?count=20&head=<frontier>`

---

## Milestone 6 — Choose Representative

- [ ] `GET /wallet/api/representatives` — server fetches and caches a curated list of known Nano representatives from the network (address, alias, weight, uptime %)
- [ ] Representative picker modal — beautiful card list, one representative per row showing alias, voting weight, and uptime badge; clicking a row selects it
- [ ] `?` tooltip button next to the "Representative" label — opens a compact inline explanation:
  - What a representative is (a node that votes on your behalf in the PoW-free consensus)
  - Why it matters (your weight counts only if your rep is online and honest)
  - That you can change it any time at no cost (a change block costs no fee, only PoW)
- [ ] `POST /wallet/change-rep` — accepts `{seed, representative}`, builds and broadcasts a `change` state block
- [ ] Confirmation screen after change: shows new representative alias and block hash
- [ ] Default representative pre-selected in the picker (e.g. a well-known community node) so users who skip the step are not left with a stale or offline rep
- [ ] Unit tests for the change-rep handler (invalid address rejected, valid block built correctly)

---

## Milestone 7 — Polish & Security

- [ ] Content-Security-Policy header on all `/wallet/*` responses
- [ ] Seed is never sent in a GET request or URL parameter
- [ ] Add `X-Content-Type-Options: nosniff` and `X-Frame-Options: DENY`
- [ ] Warn user explicitly when clipboard API is unavailable (fallback: select-all)
- [ ] Mobile layout review for wallet modal and wallet page

---

## Out of scope (by design)

- Seed storage on server — the seed stays with the user
- Multi-account HD derivation UI — index 0 only for simplicity
- Hardware wallet support
