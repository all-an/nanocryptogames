# Nano Multiplayer

A real-money multiplayer browser game powered by Go and Nano (XNO) cryptocurrency.

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
  -e POSTGRES_DB=nano_shooter \
  -p 5432:5432 \
  postgres:16
```

Connection string for your `.env`:

```
DATABASE_URL=postgres://nano:nano@localhost:5432/nano_shooter?sslmode=disable
```

### 2. Stop / remove the container

```bash
docker stop nano-multiplayer-db
docker rm nano-multiplayer-db
```

### 3. Run the server

```bash
go run ./cmd/server
```

## Environment Variables

| Variable                | Description                          | Example                          |
|-------------------------|--------------------------------------|----------------------------------|
| `DATABASE_URL`          | Postgres connection string           | see above                        |
| `NANO_RPC_PRIMARY_URL`  | Primary public Nano node             | https://nanoslo.0x.no            |
| `NANO_RPC_FALLBACK_URL` | Fallback public Nano node            | https://node.somenano.com        |
| `NANO_MASTER_SEED`      | HD wallet master seed (keep secret)  | —                                |

## Project Layout

See [ROADMAP.md](ROADMAP.md) for the full architecture and phased build plan.
