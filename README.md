# Nano Crypto Games

A collection of real-money browser games powered by Go and the Nano (XNO) cryptocurrency.

## Requirements

- Go 1.25+
- Docker (for local Postgres)

## Local Development

### 1. Start Postgres

```bash
docker run -d \
  --name nano-multiplayer-db \
  -e POSTGRES_USER=nano \
  -e POSTGRES_PASSWORD=nano \
  -e POSTGRES_DB=nano_crypto_games \
  -p 5432:5432 \
  postgres:16
```

Connection string for your `.env`:

```
DATABASE_URL=postgres://nano:nano@localhost:5432/nano_crypto_games?sslmode=disable
```

### 2. Stop / remove the container

```bash
docker stop nano-multiplayer-db
docker rm nano-multiplayer-db
```

### 3. Build and run the server

```bash
go build ./...
go run ./cmd/server
```

## Generating a Master Seed

The master seed is a 32-byte secret used to derive every player's Nano wallet via HD derivation. **Back it up — losing it means losing access to all funds held in player wallets.**

Generate one with any of these methods:

**OpenSSL (recommended):**
```bash
openssl rand -hex 32
```

**Go (one-liner):**
```bash
go run -e 'package main; import ("crypto/rand"; "encoding/hex"; "fmt"); func main() { b := make([]byte, 32); rand.Read(b); fmt.Println(hex.EncodeToString(b)) }'
```

**Python:**
```bash
python3 -c "import secrets; print(secrets.token_hex(32))"
```

The output is a 64-character hex string. Put it in your `.env`:

```
NANO_MASTER_SEED=a3f8...64hexchars...
```

> **Warning:** Never commit this value. Never reuse it across environments. If you change it, all existing player wallet addresses will change and deposited funds will become unreachable.

## Environment Variables

| Variable                | Description                          | Example                          |
|-------------------------|--------------------------------------|----------------------------------|
| `DATABASE_URL`          | Postgres connection string           | see above                        |
| `NANO_RPC_PRIMARY_URL`  | Primary public Nano node             | https://nanoslo.0x.no            |
| `NANO_RPC_FALLBACK_URL` | Fallback public Nano node            | https://node.somenano.com        |
| `NANO_MASTER_SEED`      | HD wallet master seed (keep secret)  | —                                |

## Project Layout

See [ROADMAP.md](ROADMAP.md) for the full architecture and phased build plan.
