// faucet_game.js — Faucet multiplayer mode.
// Same grid renderer as game.js; no session wallet, no deposit/withdraw.
// Kills and heals earn 0.00001 XNO paid from the server faucet wallet.

// If the player reloads the game page, send them to the lobby instead of
// spawning a duplicate player. The Navigation API reliably detects reloads.
if (performance.getEntriesByType("navigation")[0]?.type === "reload") {
  window.location.replace("/faucet/lobby");
}

const canvas = document.getElementById("game");
const ctx    = canvas.getContext("2d");

const CELL        = 40;
const COLS        = 25;
const ROWS        = 17;
const MOVE_RADIUS = 5.0;
const MOVE_SPEED  = 6;

// ── Barriers ───────────────────────────────────────────────────────────────────
// Must mirror barrierCells in physics.go exactly.

const BARRIERS = new Set();
(function () {
  function addBlock(col, row, size) {
    for (let dy = 0; dy < size; dy++)
      for (let dx = 0; dx < size; dx++)
        BARRIERS.add(`${col + dx},${row + dy}`);
  }
  addBlock(3,  2,  3); // 3×3 top-left corner
  addBlock(19, 2,  3); // 3×3 top-right corner
  addBlock(3,  12, 3); // 3×3 bottom-left corner
  addBlock(19, 12, 3); // 3×3 bottom-right corner
  addBlock(8,  7,  2); // 2×2 left mid-field
  addBlock(15, 7,  2); // 2×2 right mid-field
  addBlock(11, 2,  2); // 2×2 centre-top
  addBlock(11, 13, 2); // 2×2 centre-bottom
  addBlock(6,  10, 1); // 1×1 left flank
  addBlock(18, 10, 1); // 1×1 right flank
  addBlock(12, 5,  1); // 1×1 centre
})();

function isBarrier(gx, gy) { return BARRIERS.has(`${gx},${gy}`); }

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
const wallImpacts   = [];

// ── Ammunition ─────────────────────────────────────────────────────────────────

const MAX_AMMO    = 10;
const RELOAD_TIME = 5000; // ms

const SHOT_COOLDOWN = 600; // ms between shots

let ammo              = MAX_AMMO;
let reloading         = false;
let lastShotAt        = 0;
let reloadTimer       = null;
let reloadCountdown   = null;
let reloadPopupTimer  = null;

function updateAmmoBadge() {
  const badge = document.getElementById("ammo-badge");
  if (!badge) return;
  if (reloading) return; // countdown loop owns the badge text while reloading
  badge.textContent = `🔫 ${ammo}`;
  badge.className   = "ammo-badge" + (ammo === 0 ? " empty" : "");
}

function showReloadPopup() {
  const el = document.getElementById("reload-popup");
  if (!el) return;
  el.classList.remove("hidden");
  clearTimeout(reloadPopupTimer);
  reloadPopupTimer = setTimeout(() => el.classList.add("hidden"), 2000);
}

function startReload() {
  if (reloading) return;
  reloading = true;
  document.getElementById("reload-popup")?.classList.add("hidden");

  const badge = document.getElementById("ammo-badge");
  let remaining = Math.ceil(RELOAD_TIME / 1000);

  function tick() {
    if (!reloading) return;
    if (badge) { badge.textContent = `⟳ ${remaining}s`; badge.className = "ammo-badge reloading"; }
    remaining--;
    if (remaining > 0) reloadCountdown = setTimeout(tick, 1000);
  }
  tick();

  reloadTimer = setTimeout(() => {
    reloading = false;
    ammo = MAX_AMMO;
    updateAmmoBadge();
  }, RELOAD_TIME);
}

function resetAmmo() {
  clearTimeout(reloadTimer);
  clearTimeout(reloadCountdown);
  reloading = false;
  ammo = MAX_AMMO;
  updateAmmoBadge();
}

// ── Path helpers ───────────────────────────────────────────────────────────────

// computePath uses BFS (8-directional) to find the shortest path around barriers.
// Falls back to a straight walk only when no path exists (should not happen in practice).
function computePath(fromGX, fromGY, toGX, toGY) {
  if (fromGX === toGX && fromGY === toGY) return [];

  const startKey = `${fromGX},${fromGY}`;
  const goalKey  = `${toGX},${toGY}`;
  const parent   = { [startKey]: null };
  const queue    = [[fromGX, fromGY]];

  const dirs = [[-1,0],[1,0],[0,-1],[0,1],[-1,-1],[1,-1],[-1,1],[1,1]];

  outer: while (queue.length > 0) {
    const [cx, cy] = queue.shift();
    for (const [dx, dy] of dirs) {
      const nx = cx + dx, ny = cy + dy;
      if (nx < 0 || nx >= COLS || ny < 0 || ny >= ROWS) continue;
      if (isBarrier(nx, ny)) continue;
      const key = `${nx},${ny}`;
      if (key in parent) continue;
      parent[key] = `${cx},${cy}`;
      if (key === goalKey) break outer;
      queue.push([nx, ny]);
    }
  }

  // Reconstruct path from goal back to start.
  if (!(goalKey in parent)) {
    // No path found — return empty so the player stays put visually.
    return [];
  }
  const path = [];
  let cur = goalKey;
  while (cur !== startKey) {
    const [x, y] = cur.split(",").map(Number);
    path.unshift({ gx: x, gy: y });
    cur = parent[cur];
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
    // Local shooter already spawned the bullet in sendShoot — skip to avoid duplicate.
    if (msg.shooterID === myID) return;
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
    resetAmmo();

  } else if (msg.type === "died") {
    document.getElementById("death-overlay").classList.remove("hidden");

  } else if (msg.type === "faucet_reward") {
    showFaucetReward(msg.reason, msg.xno, msg.earned);

  } else if (msg.type === "faucet_limit") {
    showFaucetNotice(msg.message, false);

  } else if (msg.type === "faucet_err") {
    showFaucetNotice(msg.message, false);

  } else if (msg.type === "faucet_sameip") {
    showFairPlayModal(msg.message);
  }
};

// When the WebSocket closes (server restart, network drop, or tab close/reload)
// redirect back to the faucet lobby.
ws.onclose = () => {
  ctx.fillStyle = "rgba(0,0,0,0.7)";
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  ctx.fillStyle = "#fff"; ctx.font = "24px system-ui";
  ctx.textAlign = "center"; ctx.textBaseline = "middle";
  ctx.fillText("Disconnected — returning to lobby…", canvas.width / 2, canvas.height / 2);
  setTimeout(() => { window.location.replace("/faucet/lobby"); }, 1500);
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
  if (isBarrier(gx, gy)) return false;
  const dx = gx - ox, dy = gy - oy;
  return Math.sqrt(dx * dx + dy * dy) <= MOVE_RADIUS;
}
function isAdjacent(ax, ay, bx, by) {
  return Math.abs(ax - bx) <= 1 && Math.abs(ay - by) <= 1 && !(ax === bx && ay === by);
}
// extendRayToEdge extends the ray from (fromPx,fromPy) through (toPx,toPy)
// until it hits the canvas boundary. Returns the endpoint pixel position.
function extendRayToEdge(fromPx, fromPy, toPx, toPy) {
  const dx = toPx - fromPx, dy = toPy - fromPy;
  const len = Math.sqrt(dx * dx + dy * dy);
  if (len === 0) return { x: toPx, y: toPy };
  const dirX = dx / len, dirY = dy / len;
  let t = Infinity;
  if (dirX > 0) t = Math.min(t, (canvas.width  - fromPx) / dirX);
  else if (dirX < 0) t = Math.min(t, -fromPx / dirX);
  if (dirY > 0) t = Math.min(t, (canvas.height - fromPy) / dirY);
  else if (dirY < 0) t = Math.min(t, -fromPy / dirY);
  return { x: fromPx + dirX * t, y: fromPy + dirY * t };
}

// enemyOnRay returns { player, hitPx, hitPy } for the first alive enemy whose
// sprite the ray intersects, using ray-circle intersection against each player's
// animated pixel position. hitPx/hitPy is the exact entry point on the sprite edge.
// Sprite radius matches drawPlayer: CELL/2 - 2 = 18px.
function enemyOnRay(fromPx, fromPy, edgePx, edgePy) {
  const dx = edgePx - fromPx, dy = edgePy - fromPy;
  const len = Math.sqrt(dx * dx + dy * dy);
  if (len === 0) return null;
  const dirX = dx / len, dirY = dy / len;
  const r = CELL / 2 - 2; // sprite radius

  let best = null, bestT = Infinity;
  for (const p of state.players || []) {
    if (p.id === myID || p.team === myTeam || p.health === 0) continue;
    const v = playerVisuals[p.id];
    if (!v) continue;
    const ocX = v.px - fromPx, ocY = v.py - fromPy;
    const tClosest = ocX * dirX + ocY * dirY;
    if (tClosest < 0 || tClosest > len) continue;
    const perpX = ocX - tClosest * dirX, perpY = ocY - tClosest * dirY;
    const perp2 = perpX * perpX + perpY * perpY;
    if (perp2 <= r * r) {
      // Entry point on the sprite surface (front face of the circle).
      const tHit = tClosest - Math.sqrt(r * r - perp2);
      if (tHit < bestT) { bestT = tHit; best = p; }
    }
  }
  if (!best) return null;
  return { player: best, hitPx: fromPx + dirX * bestT, hitPy: fromPy + dirY * bestT };
}

// ── Keyboard ───────────────────────────────────────────────────────────────────

let lastMoveAt = 0;
const MOVE_COOLDOWN = 150;

document.addEventListener("keydown", (e) => {
  const me = myPosition();
  if (!me || me.health === 0) return;
  const now = Date.now();
  if (now - lastMoveAt < MOVE_COOLDOWN) return;
  let gx = me.gx, gy = me.gy;
  switch (e.key) {
    case "ArrowLeft":  case "a": gx--; break;
    case "ArrowRight": case "d": gx++; break;
    case "ArrowUp":    case "w": gy--; break;
    case "ArrowDown":  case "s": gy++; break;
    case "r": case "R": startReload(); return;
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
  if (!me || me.health === 0) return;
  const rect = canvas.getBoundingClientRect();
  const gx = Math.floor((e.clientX - rect.left) / CELL);
  const gy = Math.floor((e.clientY - rect.top) / CELL);

  // Clicking an adjacent incapacitated teammate heals them.
  const occupant = playerAtCell(gx, gy);
  if (occupant && occupant.id !== myID && occupant.team === myTeam &&
      occupant.health === 50 && isAdjacent(me.gx, me.gy, gx, gy)) {
    sendHelp(occupant.id);
    return;
  }

  // Ammo check — block shot if reloading, empty, or within cooldown.
  if (reloading) return;
  if (ammo === 0) { showReloadPopup(); return; }
  const now = Date.now();
  if (now - lastShotAt < SHOT_COOLDOWN) return;
  lastShotAt = now;

  const myV = playerVisuals[myID];
  if (myV) {
    const clickPx = gx * CELL + CELL / 2, clickPy = gy * CELL + CELL / 2;
    const edge = extendRayToEdge(myV.px, myV.py, clickPx, clickPy);

    const hit = enemyOnRay(myV.px, myV.py, edge.x, edge.y);
    const toPx = hit ? hit.hitPx : edge.x;
    const toPy = hit ? hit.hitPy : edge.y;
    const dist = Math.sqrt((toPx - myV.px) ** 2 + (toPy - myV.py) ** 2);
    const duration = Math.max(100, dist / 2);
    bullets.push({ fromPx: myV.px, fromPy: myV.py, toPx, toPy,
                   startTime: performance.now(), duration,
                   spawnImpactOnEnd: !!hit });

    ammo--;
    updateAmmoBadge();

    if (hit) setTimeout(() => sendShoot(hit.player.id), duration);
  }
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

function drawBarriers() {
  for (const key of BARRIERS) {
    const [gx, gy] = key.split(",").map(Number);
    const x = gx * CELL, y = gy * CELL;
    // Solid stone fill — warm grey
    ctx.fillStyle = "#7a7a8a";
    ctx.fillRect(x, y, CELL, CELL);
    // Inner face — slightly darker
    ctx.fillStyle = "#5c5c6e";
    ctx.fillRect(x + 5, y + 5, CELL - 10, CELL - 10);
    // Top & left bright bevel
    ctx.fillStyle = "#b0b0c8";
    ctx.fillRect(x, y, CELL, 5);
    ctx.fillRect(x, y, 5, CELL);
    // Bottom & right dark bevel
    ctx.fillStyle = "#2a2a3a";
    ctx.fillRect(x, y + CELL - 5, CELL, 5);
    ctx.fillRect(x + CELL - 5, y, 5, CELL);
  }
}

function draw() {
  ctx.fillStyle = "#1a1a2e";
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  updateVisuals();
  drawGrid();
  const me = myPosition();
  if (me && me.health > 0) drawReachableArea(me);
  if (hoverCell && me && me.health > 0 && isReachable(me.gx, me.gy, hoverCell.gx, hoverCell.gy)) {
    drawCellHighlight(hoverCell.gx, hoverCell.gy, "rgba(255,255,255,0.10)");
  }
  if (pending) drawCellHighlight(pending.gx, pending.gy, "rgba(74,144,217,0.30)");
  drawBarriers(); // drawn after highlights so barriers are never painted over
  for (const p of state.players || []) {
    const v = playerVisuals[p.id];
    if (v) drawPlayer(p, v.px, v.py);
  }
  drawBullets();
  drawWallImpacts();
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
  // Centre dividing line between red (left) and blue (right) halves.
  const midX = (COLS / 2) * CELL;
  ctx.save();
  ctx.strokeStyle = "rgba(180,180,220,0.25)";
  ctx.lineWidth = 2;
  ctx.setLineDash([8, 6]);
  ctx.beginPath(); ctx.moveTo(midX, 0); ctx.lineTo(midX, ROWS * CELL); ctx.stroke();
  ctx.restore();
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
  ctx.fillText(dead ? "✕" : "Ӿ", px, py);
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

function drawWallImpacts() {
  const now = performance.now();
  let i = 0;
  while (i < wallImpacts.length) {
    const f = wallImpacts[i];
    const t = Math.min(1, (now - f.startTime) / f.duration);
    const r = 6 + t * 8; // expanding ring
    ctx.save();
    ctx.globalAlpha = 1 - t;
    // Bright spark circle
    ctx.beginPath();
    ctx.arc(f.px, f.py, r, 0, Math.PI * 2);
    ctx.strokeStyle = "#FFD700";
    ctx.lineWidth = 2;
    ctx.stroke();
    // Inner white core
    ctx.beginPath();
    ctx.arc(f.px, f.py, 3 * (1 - t), 0, Math.PI * 2);
    ctx.fillStyle = "#ffffff";
    ctx.fill();
    ctx.restore();
    if (t >= 1) wallImpacts.splice(i, 1); else i++;
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
    // On barrier contact: spawn impact flash, then destroy.
    if (isBarrier(Math.floor(bpx / CELL), Math.floor(bpy / CELL))) {
      wallImpacts.push({ px: bpx, py: bpy, startTime: performance.now(), duration: 250 });
      bullets.splice(i, 1);
      continue;
    }
    ctx.beginPath(); ctx.arc(bpx, bpy, 4, 0, Math.PI * 2);
    ctx.fillStyle = "#FFD700"; ctx.fill();
    if (t >= 1) {
      if (b.spawnImpactOnEnd) {
        wallImpacts.push({ px: b.toPx, py: b.toPy, startTime: performance.now(), duration: 300 });
      }
      bullets.splice(i, 1);
    } else i++;
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

// showFairPlayModal shows a one-shot overlay asking the player to play fair.
// Subsequent calls within the same session are suppressed after the first.
let fairPlayShown = false;
function showFairPlayModal(message) {
  if (fairPlayShown) return;
  fairPlayShown = true;
  const el = document.getElementById("fairplay-modal");
  if (el) {
    document.getElementById("fairplay-msg").textContent = message;
    el.classList.remove("hidden");
  }
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
document.getElementById("fairplay-close").addEventListener("click", () => {
  document.getElementById("fairplay-modal").classList.add("hidden");
});

draw();
