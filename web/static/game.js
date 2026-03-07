// game.js — Grid-based Canvas renderer and WebSocket client.
// Server positions are authoritative grid cells (gx, gy).
// Visual positions (px, py) are interpolated each frame for smooth movement.

const canvas = document.getElementById("game");
const ctx    = canvas.getContext("2d");

// Grid constants — must match server-side physics.go values.
const CELL        = 40;
const COLS        = 25;
const ROWS        = 17;
const MOVE_RADIUS = 5.0; // must match MovementRadius in physics.go

// Lerp factor applied each frame (~60 fps).
// 0.18 gives a smooth ~200 ms glide; raise toward 1.0 for snappier feel.
const LERP = 0.18;

// Room ID embedded by the server-rendered template.
const roomID = canvas.dataset.room;

// ── State ─────────────────────────────────────────────────────────────────────

let myID      = null;
let state     = { players: [] };
let hoverCell = null;  // {gx, gy}
let pending   = null;  // {gx, gy} awaiting modal confirm

// playerVisuals stores the smooth pixel position for each player.
// key: player ID → { px, py }  (pixel centre of the circle)
const playerVisuals = {};

// ── WebSocket ─────────────────────────────────────────────────────────────────

const wsProto = location.protocol === "https:" ? "wss:" : "ws:";
const ws      = new WebSocket(`${wsProto}//${location.host}/ws/${roomID}`);

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  if (msg.type === "init") {
    myID = msg.id;
  } else if (msg.type === "state") {
    state = msg;
    // Seed visual positions for players appearing for the first time
    // so they don't slide in from (0, 0).
    for (const p of state.players) {
      if (!playerVisuals[p.id]) {
        playerVisuals[p.id] = {
          px: p.gx * CELL + CELL / 2,
          py: p.gy * CELL + CELL / 2,
        };
      }
    }
  }
};

ws.onclose = () => {
  ctx.fillStyle = "rgba(0,0,0,0.7)";
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  ctx.fillStyle = "#fff";
  ctx.font      = "24px system-ui";
  ctx.textAlign = "center";
  ctx.fillText("Disconnected", canvas.width / 2, canvas.height / 2);
};

// ── Move helpers ──────────────────────────────────────────────────────────────

function sendMove(gx, gy) {
  if (ws.readyState !== WebSocket.OPEN) return;
  ws.send(JSON.stringify({ gx, gy }));
}

// myPosition returns the local player's authoritative grid position.
// Used for movement validation — NOT for rendering (use playerVisuals for that).
function myPosition() {
  return state.players?.find(p => p.id === myID) ?? null;
}

// isReachable checks Euclidean distance, matching the server isValidMove.
function isReachable(ox, oy, gx, gy) {
  if (ox === gx && oy === gy) return false;
  const dx = gx - ox;
  const dy = gy - oy;
  return Math.sqrt(dx * dx + dy * dy) <= MOVE_RADIUS;
}

// ── Keyboard input ────────────────────────────────────────────────────────────

let lastMoveAt = 0;
const MOVE_COOLDOWN = 150; // ms — prevents key-hold spam

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

function showModal() {
  modal.classList.remove("hidden");
  btnConfirm.focus();
}

function hideModal() {
  modal.classList.add("hidden");
  pending = null;
}

btnConfirm.addEventListener("click", () => {
  if (pending) sendMove(pending.gx, pending.gy);
  hideModal();
});

btnCancel.addEventListener("click", hideModal);

document.addEventListener("keydown", (e) => {
  if (e.key === "Escape") hideModal();
});

// ── Smooth interpolation ──────────────────────────────────────────────────────

// updateVisuals advances each player's visual position toward their server grid position.
// Called once per animation frame before drawing.
function updateVisuals() {
  const activeIDs = new Set();

  for (const p of state.players || []) {
    activeIDs.add(p.id);

    const targetPx = p.gx * CELL + CELL / 2;
    const targetPy = p.gy * CELL + CELL / 2;

    const v = playerVisuals[p.id];
    if (!v) continue; // seeded in ws.onmessage; shouldn't happen

    // Lerp toward target. Snap when very close to avoid infinite micro-movement.
    const dx = targetPx - v.px;
    const dy = targetPy - v.py;
    v.px = Math.abs(dx) < 0.5 ? targetPx : v.px + dx * LERP;
    v.py = Math.abs(dy) < 0.5 ? targetPy : v.py + dy * LERP;
  }

  // Remove visuals for players who have left the room.
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

// drawGrid renders the faint grid lines over the arena.
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

// drawReachableArea fills every cell within MOVE_RADIUS with a dim tint.
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

// drawPlayer renders a Ӿ circle at the given smooth pixel position (px, py).
// px/py come from playerVisuals, not the raw grid position.
function drawPlayer(player, px, py) {
  const r    = CELL / 2 - 2;
  const isMe = player.id === myID;

  // Circle body
  ctx.beginPath();
  ctx.arc(px, py, r, 0, Math.PI * 2);
  ctx.fillStyle = player.color;
  ctx.fill();

  // White outline ring for the local player
  if (isMe) {
    ctx.strokeStyle = "#ffffff";
    ctx.lineWidth   = 2.5;
    ctx.stroke();
  }

  // Ӿ glyph
  ctx.fillStyle    = "#ffffff";
  ctx.font         = "bold 15px system-ui";
  ctx.textAlign    = "center";
  ctx.textBaseline = "middle";
  ctx.fillText("Ӿ", px, py);

  // Health bar above the circle
  drawHealthBar(px, py - r - 8, player.health);
}

// drawHealthBar renders a small bar above the player — green → amber → red.
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
