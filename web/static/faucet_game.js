// faucet_game.js — Faucet multiplayer mode.
// Same grid renderer as game.js; no session wallet, no deposit/withdraw.
// Kills and heals earn 0.00001 XNO paid from the server faucet wallet.

const canvas = document.getElementById("game");
const ctx    = canvas.getContext("2d");

const CELL        = 40;
const COLS        = 25;
const ROWS        = 17;
const MOVE_RADIUS = 5.0;
const MOVE_SPEED  = 6;

const roomID       = canvas.dataset.room;
const faucetAddr   = canvas.dataset.faucetAddress || "";

// ── State ──────────────────────────────────────────────────────────────────────

let myID   = null;
let myTeam = null;
let state  = { players: [] };
let hoverCell = null;
let pending   = null;

const playerVisuals = {};
const bullets       = [];
const healFlashes   = [];

// ── Path helpers ───────────────────────────────────────────────────────────────

function computePath(fromGX, fromGY, toGX, toGY) {
  const path = [];
  let cx = fromGX, cy = fromGY;
  while (cx !== toGX || cy !== toGY) {
    cx += Math.sign(toGX - cx);
    cy += Math.sign(toGY - cy);
    path.push({ gx: cx, gy: cy });
  }
  return path;
}

function cellCentre(gx, gy) {
  return { x: gx * CELL + CELL / 2, y: gy * CELL + CELL / 2 };
}

// ── WebSocket ──────────────────────────────────────────────────────────────────

const wsProto = location.protocol === "https:" ? "wss:" : "ws:";
const ws = new WebSocket(`${wsProto}//${location.host}/faucet/ws/${roomID}${location.search}`);

ws.onmessage = (event) => {
  const msg = JSON.parse(event.data);

  if (msg.type === "init") {
    myID   = msg.id;
    myTeam = msg.team;
    // Warn if no reward address was provided.
    if (!msg.nanoAddress) {
      document.getElementById("no-address-modal").classList.remove("hidden");
    }

  } else if (msg.type === "state") {
    for (const p of msg.players) {
      if (!playerVisuals[p.id]) {
        playerVisuals[p.id] = {
          px: p.gx * CELL + CELL / 2,
          py: p.gy * CELL + CELL / 2,
          gridX: p.gx, gridY: p.gy,
          waypoints: [],
        };
      } else {
        const v = playerVisuals[p.id];
        if (v.gridX !== p.gx || v.gridY !== p.gy) {
          const fromGX = Math.round((v.px - CELL / 2) / CELL);
          const fromGY = Math.round((v.py - CELL / 2) / CELL);
          v.waypoints = computePath(fromGX, fromGY, p.gx, p.gy);
          v.gridX = p.gx; v.gridY = p.gy;
        }
      }
    }
    state = msg;

  } else if (msg.type === "shot") {
    const sv = playerVisuals[msg.shooterID];
    const tv = playerVisuals[msg.targetID];
    if (sv && tv) {
      bullets.push({ fromPx: sv.px, fromPy: sv.py, toPx: tv.px, toPy: tv.py,
                     startTime: performance.now(), duration: 200 });
    }

  } else if (msg.type === "helped") {
    const tv = playerVisuals[msg.targetID];
    if (tv) {
      healFlashes.push({ px: tv.px, py: tv.py, startTime: performance.now(), duration: 600 });
    }

  } else if (msg.type === "roundover") {
    showRoundOver(msg.killerID, msg.prize);

  } else if (msg.type === "newround") {
    hideRoundOver();
    document.getElementById("death-overlay").classList.add("hidden");

  } else if (msg.type === "died") {
    document.getElementById("death-overlay").classList.remove("hidden");

  } else if (msg.type === "faucet_reward") {
    showFaucetReward(msg.reason, msg.xno, msg.earned);

  } else if (msg.type === "faucet_limit") {
    showFaucetNotice(msg.message, false);

  } else if (msg.type === "faucet_err") {
    showFaucetNotice(msg.message, false);
  }
};

ws.onclose = () => {
  ctx.fillStyle = "rgba(0,0,0,0.7)";
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  ctx.fillStyle = "#fff"; ctx.font = "24px system-ui";
  ctx.textAlign = "center"; ctx.textBaseline = "middle";
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

// ── Player helpers ─────────────────────────────────────────────────────────────

function myPosition() { return state.players?.find(p => p.id === myID) ?? null; }
function playerAtCell(gx, gy) { return state.players?.find(p => p.gx === gx && p.gy === gy) ?? null; }
function isReachable(ox, oy, gx, gy) {
  if (ox === gx && oy === gy) return false;
  const dx = gx - ox, dy = gy - oy;
  return Math.sqrt(dx * dx + dy * dy) <= MOVE_RADIUS;
}
function isAdjacent(ax, ay, bx, by) {
  return Math.abs(ax - bx) <= 1 && Math.abs(ay - by) <= 1 && !(ax === bx && ay === by);
}

// ── Keyboard ───────────────────────────────────────────────────────────────────

let lastMoveAt = 0;
const MOVE_COOLDOWN = 150;

document.addEventListener("keydown", (e) => {
  const me = myPosition();
  if (!me || me.health < 100) return;
  const now = Date.now();
  if (now - lastMoveAt < MOVE_COOLDOWN) return;
  let gx = me.gx, gy = me.gy;
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

// ── Mouse ──────────────────────────────────────────────────────────────────────

canvas.addEventListener("mousemove", (e) => {
  const rect = canvas.getBoundingClientRect();
  hoverCell = { gx: Math.floor((e.clientX - rect.left) / CELL), gy: Math.floor((e.clientY - rect.top) / CELL) };
});
canvas.addEventListener("mouseleave", () => { hoverCell = null; });
canvas.addEventListener("click", (e) => {
  const me = myPosition();
  if (!me || me.health < 100) return;
  const rect = canvas.getBoundingClientRect();
  const gx = Math.floor((e.clientX - rect.left) / CELL);
  const gy = Math.floor((e.clientY - rect.top) / CELL);
  if (!isReachable(me.gx, me.gy, gx, gy)) return;
  const occupant      = playerAtCell(gx, gy);
  const isOtherPlayer = occupant && occupant.id !== myID;
  const isEnemy       = isOtherPlayer && occupant.team !== myTeam && occupant.health > 0;
  const canHelp       = isOtherPlayer && occupant.team === myTeam &&
                        occupant.health === 50 && isAdjacent(me.gx, me.gy, gx, gy);
  pending = { gx, gy, targetID: isOtherPlayer ? occupant.id : null, canHelp };
  showModal({ isEnemy, canHelp });
});

// ── Modal ──────────────────────────────────────────────────────────────────────

const modal      = document.getElementById("move-modal");
const modalTitle = document.getElementById("modal-title");
const btnConfirm = document.getElementById("modal-confirm");
const btnShoot   = document.getElementById("modal-shoot");
const btnHelp    = document.getElementById("modal-help");
const btnCancel  = document.getElementById("modal-cancel");

function showModal({ isEnemy, canHelp }) {
  modalTitle.textContent = (isEnemy || canHelp) ? "What do you want?" : "Move here?";
  btnShoot.classList.toggle("hidden", !isEnemy);
  btnHelp.classList.toggle("hidden", !canHelp);
  modal.classList.remove("hidden");
  btnConfirm.focus();
}
function hideModal() { modal.classList.add("hidden"); pending = null; }

btnConfirm.addEventListener("click", () => { if (pending) sendMove(pending.gx, pending.gy); hideModal(); });
btnShoot.addEventListener("click",   () => { if (pending?.targetID) sendShoot(pending.targetID); hideModal(); });
btnHelp.addEventListener("click",    () => { if (pending?.targetID) sendHelp(pending.targetID); hideModal(); });
btnCancel.addEventListener("click", hideModal);
document.addEventListener("keydown", (e) => { if (e.key === "Escape") hideModal(); });

// ── Visuals ────────────────────────────────────────────────────────────────────

function updateVisuals() {
  const activeIDs = new Set();
  for (const p of state.players || []) {
    activeIDs.add(p.id);
    const v = playerVisuals[p.id];
    if (!v) continue;
    const wp = v.waypoints.length > 0 ? v.waypoints[0] : { gx: v.gridX, gy: v.gridY };
    const targetPx = wp.gx * CELL + CELL / 2;
    const targetPy = wp.gy * CELL + CELL / 2;
    const dx = targetPx - v.px, dy = targetPy - v.py;
    const dist = Math.sqrt(dx * dx + dy * dy);
    if (dist <= MOVE_SPEED) {
      v.px = targetPx; v.py = targetPy;
      if (v.waypoints.length > 0) v.waypoints.shift();
    } else {
      v.px += (dx / dist) * MOVE_SPEED;
      v.py += (dy / dist) * MOVE_SPEED;
    }
  }
  for (const id in playerVisuals) { if (!activeIDs.has(id)) delete playerVisuals[id]; }
}

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
  if (pending) drawCellHighlight(pending.gx, pending.gy, "rgba(74,144,217,0.30)");
  for (const p of state.players || []) {
    const v = playerVisuals[p.id];
    if (v) drawPlayer(p, v.px, v.py);
  }
  drawBullets();
  drawHealFlashes();
  requestAnimationFrame(draw);
}

function drawGrid() {
  ctx.strokeStyle = "#2a2a4a"; ctx.lineWidth = 0.5;
  for (let col = 0; col <= COLS; col++) {
    ctx.beginPath(); ctx.moveTo(col * CELL, 0); ctx.lineTo(col * CELL, ROWS * CELL); ctx.stroke();
  }
  for (let row = 0; row <= ROWS; row++) {
    ctx.beginPath(); ctx.moveTo(0, row * CELL); ctx.lineTo(COLS * CELL, row * CELL); ctx.stroke();
  }
}

function drawReachableArea(me) {
  ctx.fillStyle = "rgba(74,144,217,0.06)";
  for (let gy = 0; gy < ROWS; gy++)
    for (let gx = 0; gx < COLS; gx++)
      if (isReachable(me.gx, me.gy, gx, gy)) ctx.fillRect(gx * CELL, gy * CELL, CELL, CELL);
}

function drawCellHighlight(gx, gy, color) {
  ctx.fillStyle = color;
  ctx.fillRect(gx * CELL, gy * CELL, CELL, CELL);
}

function drawPlayer(player, px, py) {
  const r = CELL / 2 - 2;
  const isMe = player.id === myID;
  const dead = player.health === 0;
  const incap = player.health === 50;
  ctx.save();
  if (dead) ctx.globalAlpha = 0.4;
  else if (incap) ctx.globalAlpha = 0.55;
  ctx.beginPath();
  ctx.arc(px, py, r, 0, Math.PI * 2);
  ctx.fillStyle = dead ? "#666" : player.color;
  ctx.fill();
  const teamColor = player.team === "red" ? "#c0392b" : "#2471a3";
  ctx.strokeStyle = isMe ? "#ffffff" : teamColor;
  ctx.lineWidth = isMe ? 2.5 : 2;
  ctx.stroke();
  ctx.fillStyle = "#ffffff"; ctx.font = "bold 15px system-ui";
  ctx.textAlign = "center"; ctx.textBaseline = "middle";
  ctx.fillText(incap || dead ? "✕" : "Ӿ", px, py);
  ctx.restore();
  drawHealthBar(px, py - r - 8, player.health);
  if (player.nickname) {
    ctx.save();
    ctx.font = "bold 9px system-ui"; ctx.textAlign = "center"; ctx.textBaseline = "top";
    ctx.fillStyle = "rgba(0,0,0,0.55)";
    ctx.fillRect(px - 20, py + r + 3, 40, 11);
    ctx.fillStyle = player.team === "red" ? "#e88" : "#8af";
    ctx.fillText(player.nickname.slice(0, 12), px, py + r + 4);
    ctx.restore();
  }
}

function drawHealthBar(cx, cy, health) {
  const w = CELL - 4, h = 4, x = cx - w / 2;
  const pct = Math.max(0, health / 100);
  ctx.fillStyle = "#222"; ctx.fillRect(x, cy, w, h);
  ctx.fillStyle = pct > 0.5 ? "#52C07A" : pct > 0.25 ? "#F5A623" : "#E05252";
  ctx.fillRect(x, cy, w * pct, h);
}

function drawHealFlashes() {
  const now = performance.now();
  let i = 0;
  while (i < healFlashes.length) {
    const f = healFlashes[i];
    const t = Math.min(1, (now - f.startTime) / f.duration);
    ctx.save();
    ctx.globalAlpha = 1 - t;
    ctx.fillStyle = "#52C07A"; ctx.font = "bold 28px system-ui";
    ctx.textAlign = "center"; ctx.textBaseline = "middle";
    ctx.fillText("✚", f.px, f.py - 24 * t);
    ctx.restore();
    if (t >= 1) healFlashes.splice(i, 1); else i++;
  }
}

function drawBullets() {
  const now = performance.now();
  let i = 0;
  while (i < bullets.length) {
    const b = bullets[i];
    const t = Math.min(1, (now - b.startTime) / b.duration);
    const bpx = b.fromPx + (b.toPx - b.fromPx) * t;
    const bpy = b.fromPy + (b.toPy - b.fromPy) * t;
    ctx.beginPath(); ctx.arc(bpx, bpy, 4, 0, Math.PI * 2);
    ctx.fillStyle = "#FFD700"; ctx.fill();
    if (t >= 1) bullets.splice(i, 1); else i++;
  }
}

// ── Faucet UI ──────────────────────────────────────────────────────────────────

// showFaucetReward updates the earned badge and briefly flashes a toast.
function showFaucetReward(reason, xno, earned) {
  const badge = document.getElementById("faucet-earned");
  badge.classList.remove("hidden");
  badge.textContent = `Ӿ Earned: ${earned}`;

  const label = reason === "kill" ? "Kill" : "Heal";
  showFaucetNotice(`${label} reward: +${xno} XNO sent!`, true);
}

let noticeTimer = null;
function showFaucetNotice(text, ok) {
  let toast = document.getElementById("faucet-toast");
  if (!toast) {
    toast = document.createElement("div");
    toast.id = "faucet-toast";
    toast.style.cssText = `
      position:fixed;bottom:24px;left:50%;transform:translateX(-50%);
      padding:10px 20px;border-radius:8px;font-size:0.9rem;font-weight:600;
      z-index:200;pointer-events:none;transition:opacity 0.3s;
    `;
    document.body.appendChild(toast);
  }
  toast.textContent   = text;
  toast.style.background = ok ? "#1a4a2e" : "#4a1a1a";
  toast.style.color      = ok ? "#52C07A" : "#e05252";
  toast.style.border     = `1px solid ${ok ? "#2a6a3e" : "#6a2a2a"}`;
  toast.style.opacity    = "1";
  clearTimeout(noticeTimer);
  noticeTimer = setTimeout(() => { toast.style.opacity = "0"; }, 3000);
}

// ── Round over ─────────────────────────────────────────────────────────────────

function showRoundOver(killerID, prize) {
  const overlay = document.getElementById("round-overlay");
  const msg     = document.getElementById("round-msg");
  if (killerID === myID) {
    msg.textContent = `You got a kill! Faucet reward (${prize} XNO) incoming…`;
  } else {
    const killer = state.players?.find(p => p.id === killerID);
    const team   = killer ? killer.team : "a player";
    msg.textContent = `${team.charAt(0).toUpperCase() + team.slice(1)} team got a kill!`;
  }
  overlay.classList.remove("hidden");
}

function hideRoundOver() {
  document.getElementById("round-overlay").classList.add("hidden");
}

// ── Faucet donate modal ────────────────────────────────────────────────────────

document.getElementById("donate-faucet-btn").addEventListener("click", () => {
  document.getElementById("donate-faucet-modal").classList.remove("hidden");
});
document.getElementById("donate-faucet-close").addEventListener("click", () => {
  document.getElementById("donate-faucet-modal").classList.add("hidden");
});
document.getElementById("faucet-copy-btn").addEventListener("click", () => {
  navigator.clipboard.writeText(faucetAddr).then(() => {
    const el = document.getElementById("faucet-copy-confirm");
    el.classList.remove("hidden");
    setTimeout(() => el.classList.add("hidden"), 1500);
  });
});
document.getElementById("no-address-close").addEventListener("click", () => {
  document.getElementById("no-address-modal").classList.add("hidden");
});

draw();
