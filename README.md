# Nano Crypto Games

A collection of real-money browser games powered by Go and Nano (XNO) cryptocurrency.

## Requirements

- Go 1.25+
- Docker (for local Postgres)

## Local Development

### 1. Start Postgres

```bash
docker run -d \
  --name nano_crypto_games_local_db_container \
  -e POSTGRES_USER=nano \
  -e POSTGRES_PASSWORD=nano \
  -e POSTGRES_DB=nano_crypto_games_local_db \
  -p 5432:5432 \
  postgres:16
```

Connection string for your `.env`:

```
DATABASE_URL=postgres://nano:nano@localhost:5432/nano_crypto_games_local_db?sslmode=disable
```

### 2. Stop / remove the container

```bash
docker stop nano_crypto_games_local_db_container
docker rm nano_crypto_games_local_db_container
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

| Variable                | Description                          | Example / Default                                                        |
|-------------------------|--------------------------------------|--------------------------------------------------------------------------|
| `DATABASE_URL`          | Postgres connection string           | `postgres://nano:nano@localhost:5432/nano_crypto_games_local_db?sslmode=disable` |
| `DONATION_ADDRESS`      | Nano address to receive donations    | —                                                                        |
| `FAUCET_SEED`           | Seed for the faucet wallet           | —                                                                        |
| `FAUCET_TEST_MODE`      | Disable real payouts for testing     | `false`                                                                  |
| `NANO_MASTER_SEED`      | HD wallet master seed (keep secret)  | —                                                                        |
| `NANO_RPC_API_KEY`      | API key for the primary Nano node    | —                                                                        |
| `NANO_RPC_FALLBACK_URL` | Fallback public Nano node            | `https://us-1.nano.to`                                                   |
| `NANO_RPC_PRIMARY_URL`  | Primary public Nano node             | `https://rpc.nano.to`                                                    |

## Project Layout

See [ROADMAP.md](ROADMAP.md) for the full architecture and phased build plan.
