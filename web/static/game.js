// game.js — Grid-based Canvas renderer and WebSocket client.
// Server positions are authoritative grid cells (gx, gy).
// Visual movement follows a cell-by-cell path; no straight-line teleports.

const canvas = document.getElementById("game");
const ctx    = canvas.getContext("2d");

// Grid constants — must match server-side physics.go values.
const CELL        = 40;
const COLS        = 25;
const ROWS        = 17;
const MOVE_RADIUS = 5.0;   // must match MovementRadius in physics.go
const MOVE_SPEED  = 6;     // pixels per frame (~60 fps → ~100ms per cell)

const roomID = canvas.dataset.room;

// ── State ─────────────────────────────────────────────────────────────────────

let myID      = null;
let state     = { players: [] };
let hoverCell = null;
let pending   = null;

// playerVisuals: { [id]: { px, py, gridX, gridY, waypoints: [{gx,gy}] } }
// px/py   — current smooth pixel position of the circle centre
// gridX/Y — last known authoritative grid position from the server
// waypoints — queue of intermediate cell centres still to pass through
const playerVisuals = {};

// ── Path helpers ──────────────────────────────────────────────────────────────

// computePath returns the list of grid cells to step through when moving from
// (fromGX,fromGY) to (toGX,toGY).  Each step is one Chebyshev move (diagonal
// counts as one step), so the path always passes through cell centres — never
// cuts straight across the grid in pixel space.
function computePath(fromGX, fromGY, toGX, toGY) {
  const path = [];
  let cx = fromGX;
  let cy = fromGY;
  while (cx !== toGX || cy !== toGY) {
    cx += Math.sign(toGX - cx);
    cy += Math.sign(toGY - cy);
    path.push({ gx: cx, gy: cy });
  }
  return path;
}

// ── WebSocket ─────────────────────────────────────────────────────────────────

const wsProto = location.protocol === "https:" ? "wss:" : "ws:";
const ws      = new WebSocket(`${wsProto}//${location.host}/ws/${roomID}`);

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);

  if (msg.type === "init") {
    myID = msg.id;

  } else if (msg.type === "state") {
    for (const p of msg.players) {
      if (!playerVisuals[p.id]) {
        // First time we see this player — snap to exact position.
        playerVisuals[p.id] = {
          px:        p.gx * CELL + CELL / 2,
          py:        p.gy * CELL + CELL / 2,
          gridX:     p.gx,
          gridY:     p.gy,
          waypoints: [],
        };
      } else {
        const v = playerVisuals[p.id];
        if (v.gridX !== p.gx || v.gridY !== p.gy) {
          // Grid position changed — build a new path from wherever the circle
          // currently is (nearest cell) through intermediate cells to the target.
          const fromGX = Math.round((v.px - CELL / 2) / CELL);
          const fromGY = Math.round((v.py - CELL / 2) / CELL);
          v.waypoints = computePath(fromGX, fromGY, p.gx, p.gy);
          v.gridX = p.gx;
          v.gridY = p.gy;
        }
      }
    }
    state = msg;
  }
};

ws.onclose = () => {
  ctx.fillStyle = "rgba(0,0,0,0.7)";
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  ctx.fillStyle    = "#fff";
  ctx.font         = "24px system-ui";
  ctx.textAlign    = "center";
  ctx.textBaseline = "middle";
  ctx.fillText("Disconnected", canvas.width / 2, canvas.height / 2);
};

// ── Move helpers ──────────────────────────────────────────────────────────────

function sendMove(gx, gy) {
  if (ws.readyState !== WebSocket.OPEN) return;
  ws.send(JSON.stringify({ gx, gy }));
}

// myPosition returns the authoritative grid pos — used for input, not rendering.
function myPosition() {
  return state.players?.find(p => p.id === myID) ?? null;
}

function isReachable(ox, oy, gx, gy) {
  if (ox === gx && oy === gy) return false;
  const dx = gx - ox;
  const dy = gy - oy;
  return Math.sqrt(dx * dx + dy * dy) <= MOVE_RADIUS;
}

// ── Keyboard input ────────────────────────────────────────────────────────────

let lastMoveAt = 0;
const MOVE_COOLDOWN = 150;

document.addEventListener("keydown", (e) => {
  const me = myPosition();
  if (!me) return;

  const now = Date.now();
  if (now - lastMoveAt < MOVE_COOLDOWN) return;

  let gx = me.gx;
  let gy = me.gy;

  switch (e.key) {
    case "ArrowLeft":  case "a": gx--; break;
    case "ArrowRight": case "d": gx++; break;
    case "ArrowUp":    case "w": gy--; break;
    case "ArrowDown":  case "s": gy++; break;
    default: return;
  }

  e.preventDefault();
  lastMoveAt = now;
  sendMove(gx, gy);
});

// ── Mouse input ───────────────────────────────────────────────────────────────

canvas.addEventListener("mousemove", (e) => {
  const rect = canvas.getBoundingClientRect();
  hoverCell = {
    gx: Math.floor((e.clientX - rect.left) / CELL),
    gy: Math.floor((e.clientY - rect.top)  / CELL),
  };
});

canvas.addEventListener("mouseleave", () => { hoverCell = null; });

canvas.addEventListener("click", (e) => {
  const me = myPosition();
  if (!me) return;

  const rect = canvas.getBoundingClientRect();
  const gx = Math.floor((e.clientX - rect.left) / CELL);
  const gy = Math.floor((e.clientY - rect.top)  / CELL);

  if (!isReachable(me.gx, me.gy, gx, gy)) return;

  pending = { gx, gy };
  showModal();
});

// ── Modal ─────────────────────────────────────────────────────────────────────

const modal      = document.getElementById("move-modal");
const btnConfirm = document.getElementById("modal-confirm");
const btnCancel  = document.getElementById("modal-cancel");

function showModal() { modal.classList.remove("hidden"); btnConfirm.focus(); }
function hideModal() { modal.classList.add("hidden"); pending = null; }

btnConfirm.addEventListener("click", () => {
  if (pending) sendMove(pending.gx, pending.gy);
  hideModal();
});
btnCancel.addEventListener("click", hideModal);
document.addEventListener("keydown", (e) => { if (e.key === "Escape") hideModal(); });

// ── Visual update (path following) ───────────────────────────────────────────

// updateVisuals advances each player along their waypoint path at MOVE_SPEED px/frame.
// When the head of the waypoint queue is reached the next waypoint is dequeued.
function updateVisuals() {
  const activeIDs = new Set();

  for (const p of state.players || []) {
    activeIDs.add(p.id);
    const v = playerVisuals[p.id];
    if (!v) continue;

    // Next destination: first waypoint in the queue, or the final grid position.
    const wp = v.waypoints.length > 0 ? v.waypoints[0] : { gx: v.gridX, gy: v.gridY };
    const targetPx = wp.gx * CELL + CELL / 2;
    const targetPy = wp.gy * CELL + CELL / 2;

    const dx   = targetPx - v.px;
    const dy   = targetPy - v.py;
    const dist = Math.sqrt(dx * dx + dy * dy);

    if (dist <= MOVE_SPEED) {
      // Reached this waypoint — snap and advance.
      v.px = targetPx;
      v.py = targetPy;
      if (v.waypoints.length > 0) v.waypoints.shift();
    } else {
      // Step toward the waypoint at constant speed.
      v.px += (dx / dist) * MOVE_SPEED;
      v.py += (dy / dist) * MOVE_SPEED;
    }
  }

  // Drop visuals for players who left.
  for (const id in playerVisuals) {
    if (!activeIDs.has(id)) delete playerVisuals[id];
  }
}

// ── Renderer ──────────────────────────────────────────────────────────────────

function draw() {
  ctx.fillStyle = "#1a1a2e";
  ctx.fillRect(0, 0, canvas.width, canvas.height);

  updateVisuals();
  drawGrid();

  const me = myPosition();
  if (me) drawReachableArea(me);

  if (hoverCell && me && isReachable(me.gx, me.gy, hoverCell.gx, hoverCell.gy)) {
    drawCellHighlight(hoverCell.gx, hoverCell.gy, "rgba(255,255,255,0.10)");
  }
  if (pending) {
    drawCellHighlight(pending.gx, pending.gy, "rgba(74,144,217,0.30)");
  }

  for (const p of state.players || []) {
    const v = playerVisuals[p.id];
    if (v) drawPlayer(p, v.px, v.py);
  }

  requestAnimationFrame(draw);
}

function drawGrid() {
  ctx.strokeStyle = "#2a2a4a";
  ctx.lineWidth   = 0.5;
  for (let col = 0; col <= COLS; col++) {
    ctx.beginPath();
    ctx.moveTo(col * CELL, 0);
    ctx.lineTo(col * CELL, ROWS * CELL);
    ctx.stroke();
  }
  for (let row = 0; row <= ROWS; row++) {
    ctx.beginPath();
    ctx.moveTo(0,           row * CELL);
    ctx.lineTo(COLS * CELL, row * CELL);
    ctx.stroke();
  }
}

function drawReachableArea(me) {
  ctx.fillStyle = "rgba(74,144,217,0.06)";
  for (let gy = 0; gy < ROWS; gy++) {
    for (let gx = 0; gx < COLS; gx++) {
      if (isReachable(me.gx, me.gy, gx, gy)) {
        ctx.fillRect(gx * CELL, gy * CELL, CELL, CELL);
      }
    }
  }
}

function drawCellHighlight(gx, gy, color) {
  ctx.fillStyle = color;
  ctx.fillRect(gx * CELL, gy * CELL, CELL, CELL);
}

function drawPlayer(player, px, py) {
  const r    = CELL / 2 - 2;
  const isMe = player.id === myID;

  ctx.beginPath();
  ctx.arc(px, py, r, 0, Math.PI * 2);
  ctx.fillStyle = player.color;
  ctx.fill();

  if (isMe) {
    ctx.strokeStyle = "#ffffff";
    ctx.lineWidth   = 2.5;
    ctx.stroke();
  }

  ctx.fillStyle    = "#ffffff";
  ctx.font         = "bold 15px system-ui";
  ctx.textAlign    = "center";
  ctx.textBaseline = "middle";
  ctx.fillText("Ӿ", px, py);

  drawHealthBar(px, py - r - 8, player.health);
}

function drawHealthBar(cx, cy, health) {
  const w   = CELL - 4;
  const h   = 4;
  const x   = cx - w / 2;
  const pct = Math.max(0, health / 100);

  ctx.fillStyle = "#222";
  ctx.fillRect(x, cy, w, h);
  ctx.fillStyle = pct > 0.5 ? "#52C07A" : pct > 0.25 ? "#F5A623" : "#E05252";
  ctx.fillRect(x, cy, w * pct, h);
}

draw();
