## Minimal SSR web server in pure Nim.
## Uses std/asynchttpserver — zero external dependencies.

import std/asynchttpserver
import std/asyncdispatch
import std/strformat
import std/times
import std/os
import std/strutils

# ── CSS ──────────────────────────────────────────────────────────────────────
# Kept as a plain const so its braces don't interfere with strformat.

const CSS = """
* { box-sizing: border-box; margin: 0; padding: 0; }
body { font-family: system-ui, -apple-system, sans-serif; background: #0a0a0a; color: #e5e5e5; min-height: 100vh; }
nav { background: #111; border-bottom: 1px solid #222; padding: 1rem 2rem; display: flex; gap: 2rem; align-items: center; }
nav .brand { color: #fff; font-weight: 700; font-size: 1.1rem; }
nav a { color: #60a5fa; text-decoration: none; font-size: 0.95rem; }
nav a:hover { color: #93c5fd; text-decoration: underline; }
main { max-width: 760px; margin: 3rem auto; padding: 0 1.5rem; }
h1 { font-size: 2rem; margin-bottom: 0.75rem; }
h2 { font-size: 1.2rem; margin-bottom: 0.5rem; color: #d4d4d4; }
p { line-height: 1.75; color: #a3a3a3; margin-bottom: 1rem; }
a { color: #60a5fa; }
.badge { display: inline-block; background: #172554; color: #93c5fd; padding: 0.2rem 0.8rem; border-radius: 999px; font-size: 0.8rem; margin-bottom: 1.5rem; border: 1px solid #1e40af; }
.card { background: #111; border: 1px solid #1f1f1f; border-radius: 0.75rem; padding: 1.25rem 1.5rem; margin-bottom: 1rem; }
.grid { display: grid; grid-template-columns: 1fr 1fr; gap: 1rem; margin-bottom: 1.5rem; }
.stat-label { font-size: 0.8rem; color: #525252; text-transform: uppercase; letter-spacing: 0.05em; }
.stat-value { font-size: 1.5rem; font-weight: 700; color: #fff; margin-top: 0.15rem; }
.meta { font-size: 0.8rem; color: #3f3f3f; margin-top: 2.5rem; padding-top: 1rem; border-top: 1px solid #1a1a1a; }
.tag { display: inline-block; background: #1a1a1a; border: 1px solid #262626; border-radius: 0.375rem; padding: 0.15rem 0.5rem; font-size: 0.78rem; color: #a3a3a3; margin: 0.15rem; }
.highlight { color: #4ade80; }
.error-code { font-size: 5rem; font-weight: 800; color: #dc2626; line-height: 1; margin-bottom: 0.5rem; }
"""

# ── Layout ────────────────────────────────────────────────────────────────────

proc layout(title, content, path: string, ts: int64): string =
  ## Wraps page content in the shared HTML shell (nav, CSS, footer meta).
  &"""<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>{title}</title>
  <style>{CSS}</style>
</head>
<body>
  <nav>
    <span class="brand">NimSSR</span>
    <a href="/">Home</a>
    <a href="/about">About</a>
    <a href="/demo">Demo</a>
  </nav>
  <main>
    {content}
    <p class="meta">Rendered by Nim &middot; path: {path} &middot; unix: {ts}</p>
  </main>
</body>
</html>"""

# ── Pages ─────────────────────────────────────────────────────────────────────

proc pageHome(): string =
  """<span class="badge">Server-Side Rendered &middot; Nim 2.x</span>
<h1>Hello from Nim</h1>
<p>This page was assembled entirely on the server in pure Nim &mdash; no templates,
no JavaScript frameworks, no runtime.</p>
<div class="card">
  <h2>How it works</h2>
  <p>Each request hits an async HTTP server built with <code>std/asynchttpserver</code>.
  The server picks a route, formats an HTML string, and streams the response.</p>
</div>
<div class="card">
  <h2>Pages</h2>
  <p>
    <a href="/about">/about</a> &mdash; what this project is &nbsp;&middot;&nbsp;
    <a href="/demo">/demo</a> &mdash; live server data
  </p>
</div>"""

proc pageAbout(): string =
  """<span class="badge">About</span>
<h1>About NimSSR</h1>
<p>A minimal server-side rendering demo written in <span class="highlight">Nim</span>.
The binary is compiled once, copied into an Alpine Docker image, and run on Render.</p>
<div class="card">
  <h2>Stack</h2>
  <p>
    <span class="tag">Nim 2.x</span>
    <span class="tag">asynchttpserver</span>
    <span class="tag">Docker</span>
    <span class="tag">Render</span>
    <span class="tag">Alpine Linux</span>
  </p>
</div>
<div class="card">
  <h2>Why Nim?</h2>
  <p>Nim compiles to C, gives you a fast native binary, reads like Python, and ships
  a stable async HTTP server in the standard library &mdash; no dependencies needed.</p>
</div>"""

proc pageDemo(ts: int64): string =
  ## Shows live server-computed values: timestamp, UTC time, date, days since epoch.
  let dt           = fromUnix(ts).utc()
  let timeStr      = dt.format("HH:mm:ss")
  let dateStr      = dt.format("yyyy-MM-dd")
  let daysSinceEpoch = ts div 86400

  &"""<span class="badge">Live Server Data</span>
<h1>Server Demo</h1>
<p>These values were computed at request time on the server &mdash; no client-side JS.</p>
<div class="grid">
  <div class="card">
    <div class="stat-label">Unix Timestamp</div>
    <div class="stat-value">{ts}</div>
  </div>
  <div class="card">
    <div class="stat-label">UTC Time</div>
    <div class="stat-value">{timeStr}</div>
  </div>
  <div class="card">
    <div class="stat-label">UTC Date</div>
    <div class="stat-value">{dateStr}</div>
  </div>
  <div class="card">
    <div class="stat-label">Days Since Epoch</div>
    <div class="stat-value">{daysSinceEpoch}</div>
  </div>
</div>
<p>Reload the page to see the timestamp update &mdash; every request is rendered fresh.</p>"""

proc page404(path: string): string =
  &"""<div class="error-code">404</div>
<h1>Not Found</h1>
<p>No page exists at <code>{path}</code>.</p>
<p><a href="/">&#8592; Back to home</a></p>"""

# ── Handler ───────────────────────────────────────────────────────────────────

proc handler(req: Request) {.async.} =
  ## Routes the request and sends a complete HTML response.
  let path = req.url.path
  let now  = getTime().toUnix()

  var status  = Http200
  var title   = ""
  var content = ""

  case path
  of "/":
    title   = "Home — NimSSR"
    content = pageHome()
  of "/about":
    title   = "About — NimSSR"
    content = pageAbout()
  of "/demo":
    title   = "Demo — NimSSR"
    content = pageDemo(now)
  else:
    status  = Http404
    title   = "404 — NimSSR"
    content = page404(path)

  let body    = layout(title, content, path, now)
  let headers = newHttpHeaders([
    ("Content-Type",  "text/html; charset=utf-8"),
    ("Cache-Control", "no-cache"),
  ])
  await req.respond(status, body, headers)

# ── Entry point ───────────────────────────────────────────────────────────────

let portVal = parseInt(getEnv("PORT", "8080"))
let port    = Port(portVal)
var server  = newAsyncHttpServer()
echo "listening on http://0.0.0.0:" & $portVal
waitFor server.serve(port, handler)
