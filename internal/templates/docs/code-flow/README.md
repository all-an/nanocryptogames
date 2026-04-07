# Code Flow Docs

This folder contains step-by-step visual diagrams that trace what actually happens inside the server for each major feature. Unlike API docs (which describe *what* something does) or source comments (which explain *how* a single function works), a code flow doc follows a user action from the browser all the way through every layer — handler, game loop, payout goroutine, Nano RPC, database — and back again.

## Why code flow docs?

- **Onboarding**: a new contributor can see the full picture before touching any code.
- **Debugging**: when something goes wrong you can pinpoint which layer is responsible.
- **Architecture review**: visible cross-layer dependencies make it obvious when a change in one place will ripple somewhere else.

## Files

| File | Covers |
|------|--------|
| `shooter-code-flow.html` | Faucet Shooter: wallet entry → WebSocket connect → kill/heal → Nano payout. Includes bot flow comparison. |

## How to read a code flow diagram

Each diagram is a **swimlane table**: columns represent system layers (Client, Handler, Room Loop, Payout Loop, Nano RPC, Database), rows represent steps in time (top → bottom). A bubble appearing in a column means that layer is active during that step.

**Click any row** to expand the detail panel — it shows the exact function name, file path, and relevant code snippet for that step.

## Adding a new code flow

1. Create `<feature>-code-flow.html` in this folder following the structure of the shooter example.
2. Add matching `web/static/docs/code-flow/<feature>-code-flow.css` and `.js` if the feature needs distinct styling or interactivity.
3. Register the route in `cmd/server/main.go` and add a handler in `internal/handler/`.
4. Link it from `internal/templates/docs/index.html` under the Code Flows section.
5. Add a row to the table above.
