# Docker Image Creation

This document explains how the Docker image is built, what each stage does, and how to build and inspect it locally.

---

## Multi-stage build overview

```
Stage 1 (builder)             Stage 2 (runtime)
─────────────────────         ─────────────────
nimlang/nim:2.0.0-alpine  →   alpine:3.21
~500 MB                       ~15–20 MB
  ↓
  nim c -d:release --opt:speed
  ↓
  /app/server  ──── COPY ───→  /app/server
```

**Stage 1 — builder**
Uses the official Nim Alpine image which includes the Nim compiler, `gcc`, and `musl-libc`. Compiles `src/server.nim` to a native binary with release optimisations.

**Stage 2 — runtime**
Uses a bare Alpine image (~5 MB). Since both stages use musl libc, the binary runs without any extra libraries. No shell, no package manager overhead beyond Alpine's minimal base.

---

## Prerequisites

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) (Windows/macOS) or Docker Engine (Linux), version 20.10+

Verify:
```bash
docker --version
```

---

## Building the image

Run from inside `nim-version/`:

```bash
docker build -t nim-ssr .
```

To tag with a version:
```bash
docker build -t nim-ssr:0.1.0 -t nim-ssr:latest .
```

Expected output:
```
[+] Building ...
 => [builder 1/4] FROM nimlang/nim:2.0.0-alpine
 => [builder 2/4] WORKDIR /app
 => [builder 3/4] COPY . .
 => [builder 4/4] RUN nim c -d:release --opt:speed -o:server src/server.nim
 => [runtime 1/2] FROM alpine:3.21
 => [runtime 2/2] COPY --from=builder /app/server ./server
 => exporting to image
```

> First build downloads the Nim Alpine image (~500 MB) and compiles from source — roughly 2–4 minutes. Subsequent builds use Docker layer cache.

---

## Running locally

```bash
docker run --rm -p 8080:8080 nim-ssr
```

Open:

| URL | Page |
|---|---|
| `http://localhost:8080/` | Home |
| `http://localhost:8080/about` | About |
| `http://localhost:8080/demo` | Live server data |

To use a different port:
```bash
docker run --rm -p 3000:3000 -e PORT=3000 nim-ssr
```

---

## Inspecting the image

Check the final image size:
```bash
docker images nim-ssr
```

Typical output:
```
REPOSITORY   TAG       IMAGE ID       SIZE
nim-ssr      latest    a1b2c3d4e5f6   18.3MB
```

List the files inside the container:
```bash
docker run --rm nim-ssr ls -lh /app/
```

---

## Rebuilding after code changes

The `COPY src/ src/` layer invalidates when you change any source file, which triggers recompilation. Build time after a change: ~30–60 seconds.

Force a completely clean build:
```bash
docker build --no-cache -t nim-ssr .
```

---

## Pushing to a registry (required for Render)

### Docker Hub
```bash
docker tag nim-ssr YOUR_DOCKERHUB_USERNAME/nim-ssr:latest
docker push YOUR_DOCKERHUB_USERNAME/nim-ssr:latest
```

### GitHub Container Registry
```bash
echo $GITHUB_TOKEN | docker login ghcr.io -u YOUR_GITHUB_USERNAME --password-stdin
docker tag nim-ssr ghcr.io/YOUR_GITHUB_USERNAME/nim-ssr:latest
docker push ghcr.io/YOUR_GITHUB_USERNAME/nim-ssr:latest
```

> For Render's **Git-connected Docker** deployments, pushing manually is not needed — Render builds the image directly from your repository.

---

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `nimlang/nim:2.0.0-alpine` not found | Tag doesn't exist on Docker Hub | Check [hub.docker.com/r/nimlang/nim/tags](https://hub.docker.com/r/nimlang/nim/tags) for the latest Alpine tag |
| `exec format error` at runtime | Binary compiled for wrong arch | Ensure builder and runtime images share the same architecture |
| Build fails: `Error: cannot open file` | Nim can't find source | Confirm `src/server.nim` exists and `COPY src/ src/` is in the Dockerfile |
| Container exits immediately | `PORT` env var missing | Run with `-e PORT=8080` explicitly |
| `docker: 'buildx' is not a docker command` | Old Docker version | Upgrade to Docker 20.10+ |

---

## Local development (without Docker)

Install Nim via [choosenim](https://github.com/dom96/choosenim):
```bash
curl https://nim-lang.org/choosenim/init.sh -sSf | sh
```

Then run:
```bash
cd nim-version
nim c -r src/server.nim        # compile and run
# or
nimble run                     # via nimble
```
