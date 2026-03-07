// game.js — Grid-based Canvas renderer and WebSocket client.
// Movement is one square at a time; the server validates every move.

const canvas = document.getElementById("game");
const ctx    = canvas.getContext("2d");

// Grid constants — must match server-side physics.go values.
const CELL = 40;
const COLS = 25;
const ROWS = 17;
const MOVE_RADIUS = 5.0; // must match MovementRadius in physics.go

// Room ID embedded by the server-rendered template.
const roomID = canvas.dataset.room;

// ── State ────────────────────────────────────────────────────────────────────

let myID      = null;   // assigned by server "init" message
let state     = { players: [] };
let hoverCell = null;   // {gx, gy} grid cell under the mouse cursor
let pending   = null;   // {gx, gy} cell chosen via click, awaiting modal confirm

// ── WebSocket ─────────────────────────────────────────────────────────────────

const wsProto = location.protocol === "https:" ? "wss:" : "ws:";
const ws      = new WebSocket(`${wsProto}//${location.host}/ws/${roomID}`);

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);
  if (msg.type === "init") {
    myID = msg.id;
  } else if (msg.type === "state") {
    state = msg;
  }
};

ws.onclose = () => {
  ctx.fillStyle = "rgba(0,0,0,0.7)";
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  ctx.fillStyle = "#fff";
  ctx.font = "24px system-ui";
  ctx.textAlign = "center";
  ctx.fillText("Disconnected", canvas.width / 2, canvas.height / 2);
};

// ── Move helpers ──────────────────────────────────────────────────────────────

// sendMove transmits a target grid cell to the server.
// The server validates adjacency — we never trust client-side checks for game state.
function sendMove(gx, gy) {
  if (ws.readyState !== WebSocket.OPEN) return;
  ws.send(JSON.stringify({ gx, gy }));
}

// myPosition returns the local player's current grid position from the latest state.
function myPosition() {
  return state.players?.find(p => p.id === myID) ?? null;
}

// isReachable returns true when (gx,gy) is within the movement radius of (ox,oy).
// Uses Euclidean distance, matching the server-side isValidMove check.
function isReachable(ox, oy, gx, gy) {
  if (ox === gx && oy === gy) return false;
  const dx = gx - ox;
  const dy = gy - oy;
  return Math.sqrt(dx * dx + dy * dy) <= MOVE_RADIUS;
}

// ── Keyboard input ────────────────────────────────────────────────────────────

// Cooldown prevents rapid-fire moves from holding a key.
let lastMoveAt = 0;
const MOVE_COOLDOWN = 150; // ms

document.addEventListener("keydown", (e) => {
  const me = myPosition();
  if (!me) return;

  const now = Date.now();
  if (now - lastMoveAt < MOVE_COOLDOWN) return;

  let gx = me.gx;
  let gy = me.gy;

  // Keyboard moves one square at a time (still within the 5-cell radius).
  switch (e.key) {
    case "ArrowLeft":  case "a": gx--; break;
    case "ArrowRight": case "d": gx++; break;
    case "ArrowUp":    case "w": gy--; break;
    case "ArrowDown":  case "s": gy++; break;
    default: return;
  }

  // Prevent the browser from scrolling the page with arrow keys.
  e.preventDefault();

  lastMoveAt = now;
  sendMove(gx, gy);
});

// ── Mouse input ───────────────────────────────────────────────────────────────

canvas.addEventListener("mousemove", (e) => {
  const rect = canvas.getBoundingClientRect();
  hoverCell = {
    gx: Math.floor((e.clientX - rect.left)  / CELL),
    gy: Math.floor((e.clientY - rect.top)   / CELL),
  };
});

canvas.addEventListener("mouseleave", () => { hoverCell = null; });

canvas.addEventListener("click", (e) => {
  const me = myPosition();
  if (!me) return;

  const rect = canvas.getBoundingClientRect();
  const gx   = Math.floor((e.clientX - rect.left)  / CELL);
  const gy   = Math.floor((e.clientY - rect.top)   / CELL);

  if (!isReachable(me.gx, me.gy, gx, gy)) return;

  pending = { gx, gy };
  showModal();
});

// ── Modal ─────────────────────────────────────────────────────────────────────

const modal         = document.getElementById("move-modal");
const btnConfirm    = document.getElementById("modal-confirm");
const btnCancel     = document.getElementById("modal-cancel");

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

// Close modal with Escape key.
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape") hideModal();
});

// ── Renderer ──────────────────────────────────────────────────────────────────

function draw() {
  // Background fill
  ctx.fillStyle = "#1a1a2e";
  ctx.fillRect(0, 0, canvas.width, canvas.height);

  drawGrid();

  // Draw the reachable area for the local player as a subtle blue tint.
  const me = myPosition();
  if (me) {
    drawReachableArea(me);
  }

  // Highlight the hovered cell more prominently if it is reachable.
  if (hoverCell && me && isReachable(me.gx, me.gy, hoverCell.gx, hoverCell.gy)) {
    drawCellHighlight(hoverCell.gx, hoverCell.gy, "rgba(255,255,255,0.10)");
  }

  // Highlight the pending-move cell while the modal is open.
  if (pending) {
    drawCellHighlight(pending.gx, pending.gy, "rgba(74,144,217,0.30)");
  }

  for (const p of state.players || []) {
    drawPlayer(p);
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

// drawReachableArea fills every cell within MOVE_RADIUS of the local player with a dim tint.
// This gives the player a clear visual of their movement range.
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

// drawCellHighlight fills a single grid cell with a semi-transparent colour.
function drawCellHighlight(gx, gy, color) {
  ctx.fillStyle = color;
  ctx.fillRect(gx * CELL, gy * CELL, CELL, CELL);
}

// drawPlayer renders a Ӿ circle centred in its grid cell.
function drawPlayer(player) {
  const cx = player.gx * CELL + CELL / 2;
  const cy = player.gy * CELL + CELL / 2;
  const r  = CELL / 2 - 2; // slight inset so circle fits cleanly in the cell

  const isMe = player.id === myID;

  // Circle body
  ctx.beginPath();
  ctx.arc(cx, cy, r, 0, Math.PI * 2);
  ctx.fillStyle = player.color;
  ctx.fill();

  // White outline ring for the local player
  if (isMe) {
    ctx.strokeStyle = "#ffffff";
    ctx.lineWidth   = 2.5;
    ctx.stroke();
  }

  // Ӿ glyph centred in the circle
  ctx.fillStyle    = "#ffffff";
  ctx.font         = "bold 15px system-ui";
  ctx.textAlign    = "center";
  ctx.textBaseline = "middle";
  ctx.fillText("Ӿ", cx, cy);

  // Health bar above the circle
  drawHealthBar(cx, cy - r - 8, player.health);
}

// drawHealthBar renders a small coloured bar above the player.
// Colour shifts green → amber → red as health drops.
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
