# Deploying to Render

## 1. Create a PostgreSQL database

1. In the Render dashboard, go to **New → PostgreSQL**.
2. Choose a name (e.g. `nano-crypto-games-db`) and a region.
3. Click **Create Database** and wait for it to become available.
4. Copy the **Internal Database URL** — you will use it as `DATABASE_URL`.

## 2. Create a Web Service

1. Go to **New → Web Service** and connect your GitHub repository.
2. Set the following:

| Field | Value |
|---|---|
| **Runtime** | Go |
| **Build command** | `go build -o server ./cmd/server` |
| **Start command** | `./server` |
| **Instance type** | Free (or higher) |

> The server automatically picks up Render's `PORT` environment variable, so no listen address configuration is needed.

## 3. Set environment variables

Go to the service's **Environment** tab and add:

| Variable | Description | Example |
|---|---|---|
| `DATABASE_URL` | Internal DB URL from step 1 | `postgres://...` |
| `NANO_RPC_PRIMARY_URL` | Primary public Nano node | `https://nanoslo.0x.no` |
| `NANO_RPC_FALLBACK_URL` | Fallback public Nano node | `https://node.somenano.com` |
| `NANO_MASTER_SEED` | 64-char hex master seed (keep secret) | see below |
| `FAUCET_SEED` | 64-char hex faucet wallet seed | see below |

### Generating secrets

```bash
# Master seed (player wallet derivation)
openssl rand -hex 32

# Faucet seed (faucet wallet)
openssl rand -hex 32
```

Back both values up securely. **Never reuse them across environments. Never commit them.**

## 4. Static files and templates

The server serves templates from `internal/templates/` and static files from `web/static/` relative to the working directory. Render runs the binary from the repository root, so these paths resolve correctly with no extra configuration.

## 5. Deploy

Push to your connected branch (e.g. `main`). Render will automatically build and deploy on every push.

Check the **Logs** tab in the Render dashboard to confirm:
- `faucet wallet: nano_...`
- `listening on :...`

> File logging (`LOG_FILE` env var) is intentionally omitted on Render — all output goes to stdout and is captured in the Render Logs tab.

## 6. Fund the faucet

After the first deploy, copy the faucet wallet address from the logs and send some XNO to it. The faucet will start paying out rewards as soon as it has a balance.
