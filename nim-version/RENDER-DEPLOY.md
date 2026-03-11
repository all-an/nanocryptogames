# Deploying the Nim SSR server to Render

## Prerequisites

- A [Render](https://render.com) account
- The repository pushed to GitHub or GitLab

---

## 1. Create a Web Service

1. In the Render dashboard, click **New → Web Service**.
2. Connect your repository and select the branch to deploy (e.g. `main`).
3. Set the **Root Directory** to `nim-version`.
4. Set the **Environment** to **Docker** — Render detects the `Dockerfile` automatically.

| Field | Value |
|---|---|
| **Environment** | Docker |
| **Root directory** | `nim-version` |
| **Dockerfile path** | `./Dockerfile` |
| **Instance type** | Free (or higher) |

> Render injects a `PORT` environment variable at runtime. The server reads it with `getEnv("PORT", "8080")`, so no manual port config is needed.

---

## 2. Environment variables

Go to the service's **Environment** tab. No secrets are required for this project.

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | TCP port the server listens on. Render sets this automatically — do not override it. |

---

## 3. Deploy

Push to your connected branch. Render will:

1. Pull the `nim-version/` directory.
2. Run the two-stage Docker build:
   - **Stage 1** — `nimlang/nim:2.0.0-alpine` compiles the Nim source to a native binary via `nim c -d:release`.
   - **Stage 2** — copies the binary into an `alpine:3.21` image (~15–20 MB total).
3. Start the container and route HTTPS traffic to it.

First deploy takes 2–4 minutes while Nim compiles. Subsequent deploys use Docker layer cache and are faster.

---

## 4. Verify

Check the **Logs** tab in the Render dashboard. A successful start looks like:

```
listening on http://0.0.0.0:10000
```

Then open your service URL and check all three routes:

| Route | Description |
|---|---|
| `/` | Home page |
| `/about` | About page |
| `/demo` | Live server timestamp |

---

## 5. Local Docker test (optional)

```bash
cd nim-version

# Build
docker build -t nim-ssr .

# Run
docker run --rm -p 8080:8080 -e PORT=8080 nim-ssr
```

Open `http://localhost:8080` to verify before pushing.

---

## 6. Blueprint deploy via render.yaml

Place `nim-version/render.yaml` content at the **repository root** and use **New → Blueprint**:

```yaml
services:
  - type: web
    name: nim-ssr
    env: docker
    plan: free
    region: oregon
    rootDir: nim-version
    dockerfilePath: ./Dockerfile
    healthCheckPath: /
```

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| Build fails: image not found | `nimlang/nim:2.0.0-alpine` tag missing | Check available tags on [hub.docker.com/r/nimlang/nim](https://hub.docker.com/r/nimlang/nim/tags) |
| Container exits immediately | `PORT` not set or parse error | Check Render Logs; confirm env var is present |
| 502 Bad Gateway | Server not listening on `$PORT` | Verify log line shows the Render-assigned port |
| Slow first deploy | Cold Nim compile | Expected — subsequent builds use Docker cache |
