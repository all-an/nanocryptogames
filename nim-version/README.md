# nim-ssr

Minimal server-side rendered web server written in Nim. No external dependencies.

## Routes

| Path | Page |
|---|---|
| `/` | Home |
| `/about` | About |
| `/demo` | Live server timestamp |

## Run locally

```bash
nim c -r src/server.nim
```

Open `http://localhost:8080`.

## Run with Docker

```bash
docker build -t nim-ssr .
docker run --rm -p 8080:8080 nim-ssr
```

## Deploy

See [RENDER-DEPLOY.md](./RENDER-DEPLOY.md) for Render deployment instructions.
