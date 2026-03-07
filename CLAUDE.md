# CLAUDE.md

## Code Style

- Small functions — one responsibility, fits on screen
- Small structs — group only what belongs together
- Comment every exported function, type, and non-obvious block
- Follow standard Go conventions (gofmt, golint)

## Testing

- Write unit tests for every function with logic
- Table-driven tests preferred
- Use interfaces and mocks to isolate dependencies (especially NanoRPC and DB)

## General

- Prefer clarity over cleverness
- No premature abstractions — solve the problem in front of you
- Keep dependencies minimal
