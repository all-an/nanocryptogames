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
let myTeam    = null;  // "red" or "blue" — set on init
let currentBalanceRaw = BigInt(0); // real on-chain balance in raw units (updated via WS)
let state     = { players: [] };
let hoverCell = null;
let pending   = null;  // { gx, gy, targetID, canHelp } — the cell the player clicked

// playerVisuals: { [id]: { px, py, gridX, gridY, waypoints: [{gx,gy}] } }
// px/py   — current smooth pixel position of the circle centre
// gridX/Y — last known authoritative grid position from the server
// waypoints — queue of intermediate cell centres still to pass through
const playerVisuals = {};

// bullets: [{fromPx, fromPy, toPx, toPy, startTime, duration}]
// Each bullet animates from shooter to target over `duration` ms.
const bullets = [];

// healFlashes: [{px, py, startTime, duration}]
// Each flash shows a green cross at a healed player's position.
const healFlashes = [];

// ── Path helpers ──────────────────────────────────────────────────────────────

// computePath returns the list of grid cells to step through when moving from
// (fromGX,fromGY) to (toGX,toGY).  Each step is one Chebyshev move (diagonal
// counts as one step), so the path always passes through cell centres.
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

// cellCentre returns the canvas pixel coordinates of a grid cell's centre.
function cellCentre(gx, gy) {
  return { x: gx * CELL + CELL / 2, y: gy * CELL + CELL / 2 };
}

// ── WebSocket ─────────────────────────────────────────────────────────────────

const wsProto = location.protocol === "https:" ? "wss:" : "ws:";
const ws      = new WebSocket(`${wsProto}//${location.host}/ws/${roomID}`);

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);

  if (msg.type === "init") {
    myID   = msg.id;
    myTeam = msg.team;
    if (msg.nanoAddress) showDepositPanel(msg.nanoAddress);
    updateBalance("0.000000");

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

  } else if (msg.type === "shot") {
    // Spawn a bullet animation from the shooter to the target.
    const sv = playerVisuals[msg.shooterID];
    const tv = playerVisuals[msg.targetID];
    if (sv && tv) {
      bullets.push({
        fromPx:    sv.px,
        fromPy:    sv.py,
        toPx:      tv.px,
        toPy:      tv.py,
        startTime: performance.now(),
        duration:  200,
      });
    }

  } else if (msg.type === "helped") {
    // Flash a green cross at the healed player's position.
    const tv = playerVisuals[msg.targetID];
    if (tv) {
      healFlashes.push({ px: tv.px, py: tv.py, startTime: performance.now(), duration: 600 });
    }

  } else if (msg.type === "balance") {
    updateBalance(msg.xno);
    if (msg.raw) currentBalanceRaw = BigInt(msg.raw);

  } else if (msg.type === "roundover") {
    showRoundOver(msg.killerID, msg.prize);

  } else if (msg.type === "newround") {
    hideRoundOver();
    document.getElementById("death-overlay").classList.add("hidden");

  } else if (msg.type === "died") {
    document.getElementById("death-overlay").classList.remove("hidden");

  } else if (msg.type === "withdraw_ok") {
    showWithdrawResult(true,
      `Sent ${msg.xno} XNO to ${msg.toAddress.slice(0, 16)}…`);

  } else if (msg.type === "withdraw_err") {
    showWithdrawResult(false, msg.message);

  } else if (msg.type === "donate_ok") {
    showDonateResult(true, `Thank you! Donated ${msg.xno} XNO`);

  } else if (msg.type === "donate_err") {
    showDonateResult(false, msg.message);
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

// ── Send helpers ───────────────────────────────────────────────────────────────

function sendMove(gx, gy) {
  if (ws.readyState !== WebSocket.OPEN) return;
  ws.send(JSON.stringify({ action: "move", gx, gy }));
}

function sendShoot(targetID) {
  if (ws.readyState !== WebSocket.OPEN) return;
  ws.send(JSON.stringify({ action: "shoot", targetID }));
}

function sendHelp(targetID) {
  if (ws.readyState !== WebSocket.OPEN) return;
  ws.send(JSON.stringify({ action: "help", targetID }));
}

// ── Player lookup helpers ──────────────────────────────────────────────────────

// myPosition returns the authoritative player state for the local player.
function myPosition() {
  return state.players?.find(p => p.id === myID) ?? null;
}

// playerAtCell returns the player occupying (gx, gy), or null.
function playerAtCell(gx, gy) {
  return state.players?.find(p => p.gx === gx && p.gy === gy) ?? null;
}

function isReachable(ox, oy, gx, gy) {
  if (ox === gx && oy === gy) return false;
  const dx = gx - ox;
  const dy = gy - oy;
  return Math.sqrt(dx * dx + dy * dy) <= MOVE_RADIUS;
}

// isAdjacent returns true when two grid positions are within Chebyshev distance 1.
function isAdjacent(ax, ay, bx, by) {
  return Math.abs(ax - bx) <= 1 && Math.abs(ay - by) <= 1 && !(ax === bx && ay === by);
}

// ── Keyboard input ────────────────────────────────────────────────────────────

let lastMoveAt = 0;
const MOVE_COOLDOWN = 150;

document.addEventListener("keydown", (e) => {
  const me = myPosition();
  if (!me || me.health < 100) return; // only healthy players can move

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
  if (!me || me.health < 100) return; // only healthy players can act

  const rect = canvas.getBoundingClientRect();
  const gx = Math.floor((e.clientX - rect.left) / CELL);
  const gy = Math.floor((e.clientY - rect.top)  / CELL);

  if (!isReachable(me.gx, me.gy, gx, gy)) return;

  const occupant      = playerAtCell(gx, gy);
  const isOtherPlayer = occupant && occupant.id !== myID;
  const isEnemy       = isOtherPlayer && occupant.team !== myTeam && occupant.health > 0;
  const canHelp       = isOtherPlayer && occupant.team === myTeam &&
                        occupant.health === 50 &&
                        isAdjacent(me.gx, me.gy, gx, gy);

  pending = { gx, gy, targetID: isOtherPlayer ? occupant.id : null, canHelp };
  showModal({ isEnemy, canHelp });
});

// ── Modal ─────────────────────────────────────────────────────────────────────

const modal      = document.getElementById("move-modal");
const modalTitle = document.getElementById("modal-title");
const btnConfirm = document.getElementById("modal-confirm");
const btnShoot   = document.getElementById("modal-shoot");
const btnHelp    = document.getElementById("modal-help");
const btnCancel  = document.getElementById("modal-cancel");

function showModal({ isEnemy, canHelp }) {
  if (isEnemy || canHelp) {
    modalTitle.textContent = "What do you want?";
  } else {
    modalTitle.textContent = "Move here?";
  }
  btnShoot.classList.toggle("hidden", !isEnemy);
  btnHelp.classList.toggle("hidden", !canHelp);
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

btnShoot.addEventListener("click", () => {
  if (pending?.targetID) sendShoot(pending.targetID);
  hideModal();
});

btnHelp.addEventListener("click", () => {
  if (pending?.targetID) sendHelp(pending.targetID);
  hideModal();
});

btnCancel.addEventListener("click", hideModal);
document.addEventListener("keydown", (e) => { if (e.key === "Escape") hideModal(); });

// ── Visual update (path following + bullets) ──────────────────────────────────

// updateVisuals advances each player along their waypoint path at MOVE_SPEED px/frame.
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
  if (me && me.health === 100) drawReachableArea(me);

  if (hoverCell && me && me.health === 100 && isReachable(me.gx, me.gy, hoverCell.gx, hoverCell.gy)) {
    drawCellHighlight(hoverCell.gx, hoverCell.gy, "rgba(255,255,255,0.10)");
  }
  if (pending) {
    drawCellHighlight(pending.gx, pending.gy, "rgba(74,144,217,0.30)");
  }

  for (const p of state.players || []) {
    const v = playerVisuals[p.id];
    if (v) drawPlayer(p, v.px, v.py);
  }

  drawBullets();
  drawHealFlashes();

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

// drawPlayer renders a player circle with health-dependent appearance.
// Healthy (100): full colour + Ӿ symbol.
// Incapacitated (50): dimmed circle + ✕ symbol.
// Dead (0): grey circle + ✕ symbol (visible for 2s grace period).
function drawPlayer(player, px, py) {
  const r    = CELL / 2 - 2;
  const isMe = player.id === myID;

  const dead          = player.health === 0;
  const incapacitated = player.health === 50;

  ctx.save();

  if (dead) {
    ctx.globalAlpha = 0.4;
  } else if (incapacitated) {
    ctx.globalAlpha = 0.55;
  }

  ctx.beginPath();
  ctx.arc(px, py, r, 0, Math.PI * 2);
  ctx.fillStyle = dead ? "#666" : player.color;
  ctx.fill();

  // Team ring: white for self, team colour for others.
  const teamColor = player.team === "red" ? "#c0392b" : "#2471a3";
  ctx.strokeStyle = isMe ? "#ffffff" : teamColor;
  ctx.lineWidth   = isMe ? 2.5 : 2;
  ctx.stroke();

  ctx.fillStyle    = "#ffffff";
  ctx.font         = "bold 15px system-ui";
  ctx.textAlign    = "center";
  ctx.textBaseline = "middle";
  ctx.fillText(incapacitated || dead ? "✕" : "Ӿ", px, py);

  ctx.restore();

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

// drawHealFlashes renders a fading green cross at the position of a recently healed player.
function drawHealFlashes() {
  const now = performance.now();
  let i = 0;
  while (i < healFlashes.length) {
    const f     = healFlashes[i];
    const t     = Math.min(1, (now - f.startTime) / f.duration);
    const alpha = 1 - t;

    ctx.save();
    ctx.globalAlpha = alpha;
    ctx.fillStyle   = "#52C07A";
    ctx.font        = "bold 28px system-ui";
    ctx.textAlign   = "center";
    ctx.textBaseline = "middle";
    ctx.fillText("✚", f.px, f.py - 24 * t); // rises slightly as it fades

    ctx.restore();

    if (t >= 1) {
      healFlashes.splice(i, 1);
    } else {
      i++;
    }
  }
}

// drawBullets renders all active bullet animations, removing finished ones.
function drawBullets() {
  const now = performance.now();
  let i = 0;
  while (i < bullets.length) {
    const b   = bullets[i];
    const t   = Math.min(1, (now - b.startTime) / b.duration);
    const bpx = b.fromPx + (b.toPx - b.fromPx) * t;
    const bpy = b.fromPy + (b.toPy - b.fromPy) * t;

    ctx.beginPath();
    ctx.arc(bpx, bpy, 4, 0, Math.PI * 2);
    ctx.fillStyle = "#FFD700";
    ctx.fill();

    if (t >= 1) {
      bullets.splice(i, 1); // animation complete
    } else {
      i++;
    }
  }
}

// ── Deposit panel ─────────────────────────────────────────────────────────────

// updateBalance refreshes the balance display in the deposit panel.
function updateBalance(xno) {
  const el = document.getElementById("deposit-balance");
  if (el) el.textContent = `${xno} XNO`;
}

// showRoundOver displays the round-over overlay with winner info and countdown.
function showRoundOver(killerID, prize) {
  const overlay = document.getElementById("round-overlay");
  const msg     = document.getElementById("round-msg");
  const isMe    = killerID === myID;

  if (isMe) {
    msg.textContent = `You earned +${prize} XNO this round!`;
  } else {
    const killer = state.players?.find(p => p.id === killerID);
    const team   = killer ? killer.team : "a player";
    msg.textContent = `${team.charAt(0).toUpperCase() + team.slice(1)} team got a kill! Round restarting…`;
  }

  overlay.classList.remove("hidden");
}

function hideRoundOver() {
  document.getElementById("round-overlay").classList.add("hidden");
}

// ── Donation modal ────────────────────────────────────────────────────────────

let selectedDonateRaw = BigInt(0);

function openDonateModal() {
  const info = document.getElementById("donate-balance-info");
  const confirm = document.getElementById("donate-confirm");
  const selectedAmt = document.getElementById("donate-selected-amt");
  const status = document.getElementById("donate-status");

  selectedDonateRaw = BigInt(0);
  confirm.disabled = true;
  selectedAmt.textContent = "";
  status.classList.add("hidden");
  document.querySelectorAll(".donate-preset-btn").forEach(b => b.classList.remove("selected"));

  if (currentBalanceRaw <= BigInt(0)) {
    info.textContent = "Your session balance is empty. Deposit Nano first.";
    document.querySelectorAll(".donate-preset-btn").forEach(b => b.disabled = true);
  } else {
    const balXNO = document.getElementById("deposit-balance").textContent;
    info.textContent = `Session balance: ${balXNO}`;
    document.querySelectorAll(".donate-preset-btn").forEach(b => b.disabled = false);
  }

  document.getElementById("donate-modal").classList.remove("hidden");
}

function closeDonateModal() {
  document.getElementById("donate-modal").classList.add("hidden");
}

function selectDonatePreset(pct) {
  selectedDonateRaw = (currentBalanceRaw * BigInt(pct)) / BigInt(100);
  // Format for display: raw / 10^24 gives us 6 decimal places (out of 10^30 raw = 1 XNO)
  const divisor = BigInt("1000000000000000000000000"); // 10^24
  const whole = selectedDonateRaw / BigInt("1000000000000000000000000000000"); // 10^30
  const rem = selectedDonateRaw % BigInt("1000000000000000000000000000000");
  const frac = rem / divisor;
  const xno = `${whole}.${frac.toString().padStart(6, "0")}`;
  document.getElementById("donate-selected-amt").textContent = `${xno} XNO`;
  document.getElementById("donate-confirm").disabled = selectedDonateRaw <= BigInt(0);
  document.querySelectorAll(".donate-preset-btn").forEach(b =>
    b.classList.toggle("selected", Number(b.dataset.pct) === pct));
}

function sendDonate() {
  if (ws.readyState !== WebSocket.OPEN || selectedDonateRaw <= BigInt(0)) return;
  const confirm = document.getElementById("donate-confirm");
  confirm.disabled = true;
  confirm.textContent = "Sending donation…";
  ws.send(JSON.stringify({ action: "donate", amountRaw: selectedDonateRaw.toString() }));
}

function showDonateResult(ok, text) {
  const confirm = document.getElementById("donate-confirm");
  const status = document.getElementById("donate-status");
  confirm.disabled = false;
  confirm.textContent = "Send Donation";
  status.textContent = text;
  status.className = `donate-status-msg ${ok ? "ok" : "err"}`;
  status.classList.remove("hidden");
  if (ok) setTimeout(closeDonateModal, 3000);
}

document.getElementById("donate-trigger").addEventListener("click", openDonateModal);
document.getElementById("donate-cancel").addEventListener("click", closeDonateModal);
document.getElementById("donate-confirm").addEventListener("click", sendDonate);
document.querySelectorAll(".donate-preset-btn").forEach(btn => {
  btn.addEventListener("click", () => selectDonatePreset(Number(btn.dataset.pct)));
});

// ── Withdraw ──────────────────────────────────────────────────────────────────

function sendWithdraw() {
  if (ws.readyState !== WebSocket.OPEN) return;
  const btn = document.getElementById("withdraw-btn");
  btn.disabled = true;
  btn.textContent = "Sending…";
  ws.send(JSON.stringify({ action: "withdraw" }));
}

function showWithdrawResult(ok, text) {
  const btn    = document.getElementById("withdraw-btn");
  const status = document.getElementById("withdraw-status");
  btn.disabled    = false;
  btn.textContent = "Withdraw";
  status.textContent = text;
  status.className   = ok ? "withdraw-status withdraw-status--ok" : "withdraw-status withdraw-status--err";
  status.classList.remove("hidden");
  setTimeout(() => status.classList.add("hidden"), 6000);
}

// showDepositPanel populates the sidebar and funds modal with the session address.
function showDepositPanel(address) {
  document.getElementById("deposit-waiting").classList.add("hidden");
  document.getElementById("deposit-ready").classList.remove("hidden");

  // Populate the Add Funds modal address fields.
  document.getElementById("funds-address").textContent = address;
  document.getElementById("funds-nault-link").href = `https://nault.cc/account/${address}`;

  document.getElementById("funds-copy-btn").addEventListener("click", () => {
    navigator.clipboard.writeText(address).then(() => {
      const el = document.getElementById("funds-copy-confirm");
      el.classList.remove("hidden");
      setTimeout(() => el.classList.add("hidden"), 1500);
    });
  });

  document.getElementById("add-funds-btn").addEventListener("click", () =>
    document.getElementById("funds-modal").classList.remove("hidden"));
  document.getElementById("funds-close-btn").addEventListener("click", () =>
    document.getElementById("funds-modal").classList.add("hidden"));

  document.getElementById("withdraw-btn").addEventListener("click", sendWithdraw);
}

draw();
