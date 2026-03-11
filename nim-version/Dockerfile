# ---------------------------------------------------------------------------
# Stage 1 — Build
# nimlang/nim:2.0.0-alpine includes the Nim compiler and musl-gcc.
# The result is a native binary linked against musl libc.
# ---------------------------------------------------------------------------
FROM nimlang/nim:2.0.0-alpine AS builder

WORKDIR /app

COPY nim-ssr.nimble .
COPY src/ src/

# -d:release strips debug info; --opt:speed enables full C-level optimisation.
RUN nim c -d:release --opt:speed -o:server src/server.nim

# ---------------------------------------------------------------------------
# Stage 2 — Runtime
# Alpine uses musl libc, matching what the builder produced.
# Final image is ~15–20 MB (Alpine base + Nim binary).
# ---------------------------------------------------------------------------
FROM alpine:3.21

WORKDIR /app

COPY --from=builder /app/server ./server

EXPOSE 8080
ENV PORT=8080

CMD ["./server"]
