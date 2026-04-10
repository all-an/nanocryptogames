// faucet_npc_bots.js — Practice mode: player (red) vs 3 blue bots.
// Runs client-side. Killing a bot pays a faucet reward if a Nano address is provided.
// Same grid, barriers, health system, and controls as faucet_game.js.

const canvas = document.getElementById("game");
const ctx    = canvas.getContext("2d");

const CELL        = 40;
const COLS        = 25;
const ROWS        = 17;
const MOVE_RADIUS = 5.0;
const MOVE_SPEED  = 6;

const DOOR_GX = 24; // far-right portal — step on it to advance to stage 2
const DOOR_GY = 8;

let stage = 1;

const MAX_AMMO      = 10;
const RELOAD_TIME   = 5000;
const SHOT_COOLDOWN = 600;

const BOT_MOVE_INTERVAL  = 1100; // ms between bot steps
const BOT_SHOT_COOLDOWN  = 950;  // ms between shots for each individual bot
const BOT_GLOBAL_SHOT_CD = 1400; // ms minimum gap between any two bots shooting

// ── Barriers ───────────────────────────────────────────────────────────────────
// Must mirror barrierCells in physics.go and BARRIERS in faucet_game.js exactly.

const BARRIERS = new Set();
(function () {
  function addBlock(col, row, size) {
    for (let dy = 0; dy < size; dy++)
      for (let dx = 0; dx < size; dx++)
        BARRIERS.add(`${col + dx},${row + dy}`);
  }
  addBlock(3,  2,  3);
  addBlock(19, 2,  3);
  addBlock(3,  12, 3);
  addBlock(19, 12, 3);
  addBlock(8,  7,  2);
  addBlock(15, 7,  2);
  addBlock(11, 2,  2);
  addBlock(11, 13, 2);
  addBlock(6,  10, 1);
  addBlock(18, 10, 1);
  addBlock(12, 5,  1);
})();

function isBarrier(gx, gy) { return BARRIERS.has(`${gx},${gy}`); }

// ── Faucet reward ──────────────────────────────────────────────────────────────
// Address is read from the ?address= query param set by the lobby.

const nanoAddress = new URLSearchParams(location.search).get("address") || "";
let totalEarned = 0; // count of rewards paid this session (display only)

async function claimBotKillReward() {
  if (!nanoAddress) {
    showBotToast("Add a Nano address in the lobby to earn rewards.", false);
    return;
  }
  try {
    const res = await fetch("/shooter/bots/reward", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ address: nanoAddress }),
    });
    if (res.ok) {
      const data = await res.json();
      totalEarned++;
      showBotToast(`Bot kill reward: +${data.xno} XNO sent!`, true);
      const badge = document.getElementById("faucet-earned");
      if (badge) {
        badge.classList.remove("hidden");
        badge.textContent = `Ӿ Earned: ${(totalEarned * 0.00001).toFixed(5)} XNO`;
      }
    } else if (res.status === 429) {
      showBotToast("Daily faucet limit reached.", false);
    } else {
      const text = await res.text().catch(() => "");
      showBotToast(`Reward failed: ${text || res.status}`, false);
    }
  } catch (err) {
    showBotToast(`Reward error: ${err.message}`, false);
  }
}

let botToastTimer = null;
function showBotToast(text, ok) {
  let toast = document.getElementById("bot-toast");
  if (!toast) {
    toast = document.createElement("div");
    toast.id = "bot-toast";
    toast.style.cssText = `
      position:fixed;bottom:24px;left:50%;transform:translateX(-50%);
      padding:10px 20px;border-radius:8px;font-size:0.9rem;font-weight:600;
      z-index:200;pointer-events:none;transition:opacity 0.3s;
    `;
    document.body.appendChild(toast);
  }
  toast.textContent = text;
  toast.style.background = ok ? "#1a4a2e" : "#4a1a1a";
  toast.style.color      = ok ? "#52C07A" : "#e05252";
  toast.style.border     = `1px solid ${ok ? "#2a6a3e" : "#6a2a2a"}`;
  toast.style.opacity    = "1";
  clearTimeout(botToastTimer);
  botToastTimer = setTimeout(() => { toast.style.opacity = "0"; }, 3000);
}

// ── Player state ───────────────────────────────────────────────────────────────

const ME = {
  id: "player", gx: 1, gy: 8, health: 99,
  team: "red", color: "#E05252", nickname: "You",
  ammo: MAX_AMMO, reloading: false, lastShotAt: 0,
  spawnGX: 1, spawnGY: 8,
};

// ── Bot state ──────────────────────────────────────────────────────────────────

const BOT_DEFS = [
  { id: "bot1", spawnGX: 23, spawnGY: 1,  color: "#4A90D9", nickname: "Bot 1" },
  { id: "bot2", spawnGX: 23, spawnGY: 8,  color: "#1ABC9C", nickname: "Bot 2" },
  { id: "bot3", spawnGX: 22, spawnGY: 15, color: "#9B59B6", nickname: "Bot 3" },
];

// Stage-2 extra bot: spawns top-right and activates immediately alongside bot 3.
const BOT4_DEF = { id: "bot0", spawnGX: 24, spawnGY: 1, color: "#FF6B35", nickname: "Bot 0" };

let bots = [];

function initBots() {
  const now = Date.now();
  const stagger = Math.floor(BOT_MOVE_INTERVAL / BOT_DEFS.length);
  bots = BOT_DEFS.map((def, i) => ({
    ...def,
    gx: def.spawnGX, gy: def.spawnGY,
    health: 66, ammo: MAX_AMMO,
    reloading: false, lastShotAt: 0,
    // Each bot gets a different initial offset so they move one at a time.
    lastMoveAt: now - BOT_MOVE_INTERVAL + i * stagger,
    team: "blue", forcedActive: false,
  }));
}

// ── Visuals & effects ──────────────────────────────────────────────────────────

const playerVisuals = {};
const bullets       = [];
const healFlashes   = [];
const wallImpacts   = [];

function ensureVisual(entity) {
  if (!playerVisuals[entity.id]) {
    playerVisuals[entity.id] = {
      px: entity.gx * CELL + CELL / 2,
      py: entity.gy * CELL + CELL / 2,
      gridX: entity.gx, gridY: entity.gy,
      waypoints: [],
    };
  }
}

// ── Heal items ─────────────────────────────────────────────────────────────────

const HEAL_ITEM_COUNT   = 4;
const HEAL_ITEM_RESPAWN = 15000;

let healItems = []; // array of { gx, gy }

function spawnHealItems(count) {
  for (let i = 0; i < count; i++) spawnOneHealItem();
}

function spawnOneHealItem() {
  const occupied = new Set(healItems.map(h => `${h.gx},${h.gy}`));
  const free = [];
  for (let gy = 0; gy < ROWS; gy++)
    for (let gx = 0; gx < COLS; gx++)
      if (!isBarrier(gx, gy) && !occupied.has(`${gx},${gy}`))
        free.push({ gx, gy });
  if (free.length === 0) return;
  healItems.push(free[Math.floor(Math.random() * free.length)]);
}

function checkHealPickup(entity) {
  const idx = healItems.findIndex(h => h.gx === entity.gx && h.gy === entity.gy);
  if (idx === -1) return;
  healItems.splice(idx, 1);
  entity.health = Math.min(99, entity.health + 33);
  setTimeout(spawnOneHealItem, HEAL_ITEM_RESPAWN);
}

// ── Stage door ─────────────────────────────────────────────────────────────────

// checkDoorEntry is called after every player move.
// Stepping on the door cell while in stage 1 advances the game to stage 2.
function checkDoorEntry() {
  if (stage !== 1 || ME.gx !== DOOR_GX || ME.gy !== DOOR_GY) return;
  advanceToStage2();
}

// advanceToStage2 unlocks stage 2: spawns Bot 0 who activates immediately with bot 3.
function advanceToStage2() {
  stage = 2;
  showBotToast("⚠ Stage 2 — Bot 0 has entered!", true);
  const bot0 = {
    ...BOT4_DEF,
    gx: BOT4_DEF.spawnGX, gy: BOT4_DEF.spawnGY,
    health: 66, ammo: MAX_AMMO,
    reloading: false, lastShotAt: 0,
    lastMoveAt: Date.now() - BOT_MOVE_INTERVAL,
    team: "blue",
  };
  bots.push(bot0);
  ensureVisual(bot0);
}

// ── Ammo badge ─────────────────────────────────────────────────────────────────

let reloadPopupTimer  = null;
let reloadCountdown   = null;
let reloadTimer       = null;

function updateAmmoBadge() {
  const badge = document.getElementById("ammo-badge");
  if (!badge || ME.reloading) return;
  badge.textContent = `🔫 ${ME.ammo}`;
  badge.className   = "ammo-badge" + (ME.ammo === 0 ? " empty" : "");
}

function showReloadPopup() {
  const el = document.getElementById("reload-popup");
  if (!el) return;
  el.classList.remove("hidden");
  clearTimeout(reloadPopupTimer);
  reloadPopupTimer = setTimeout(() => el.classList.add("hidden"), 2000);
}

function startReload() {
  if (ME.reloading) return;
  ME.reloading = true;
  document.getElementById("reload-popup")?.classList.add("hidden");
  const badge = document.getElementById("ammo-badge");
  let remaining = Math.ceil(RELOAD_TIME / 1000);
  function tick() {
    if (!ME.reloading) return;
    if (badge) { badge.textContent = `⟳ ${remaining}s`; badge.className = "ammo-badge reloading"; }
    remaining--;
    if (remaining > 0) reloadCountdown = setTimeout(tick, 1000);
  }
  tick();
  reloadTimer = setTimeout(() => {
    ME.reloading = false;
    ME.ammo = MAX_AMMO;
    updateAmmoBadge();
  }, RELOAD_TIME);
}

// ── Path helpers ───────────────────────────────────────────────────────────────

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
  if (!(goalKey in parent)) return [];
  const path = [];
  let cur = goalKey;
  while (cur !== startKey) {
    const [x, y] = cur.split(",").map(Number);
    path.unshift({ gx: x, gy: y });
    cur = parent[cur];
  }
  return path;
}

// ── Line-of-sight (Bresenham) ──────────────────────────────────────────────────

function hasLineOfSight(x0, y0, x1, y1) {
  const dx = Math.abs(x1 - x0), dy = Math.abs(y1 - y0);
  const sx = x0 < x1 ? 1 : -1, sy = y0 < y1 ? 1 : -1;
  let err = dx - dy, x = x0, y = y0;
  while (true) {
    if (!(x === x0 && y === y0) && !(x === x1 && y === y1))
      if (isBarrier(x, y)) return false;
    if (x === x1 && y === y1) return true;
    const e2 = 2 * err;
    if (e2 > -dy) { err -= dy; x += sx; }
    if (e2 < dx)  { err += dx; y += sy; }
  }
}

// ── Ray helpers ────────────────────────────────────────────────────────────────

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

// botOnRay returns the first alive bot whose sprite the ray intersects.
function botOnRay(fromPx, fromPy, edgePx, edgePy) {
  const dx = edgePx - fromPx, dy = edgePy - fromPy;
  const len = Math.sqrt(dx * dx + dy * dy);
  if (len === 0) return null;
  const dirX = dx / len, dirY = dy / len;
  const r = CELL / 2 - 2;
  let best = null, bestT = Infinity;
  for (const bot of bots) {
    if (bot.health === 0) continue;
    const v = playerVisuals[bot.id];
    if (!v) continue;
    const ocX = v.px - fromPx, ocY = v.py - fromPy;
    const tClosest = ocX * dirX + ocY * dirY;
    if (tClosest < 0 || tClosest > len) continue;
    const perpX = ocX - tClosest * dirX, perpY = ocY - tClosest * dirY;
    const perp2 = perpX * perpX + perpY * perpY;
    if (perp2 <= r * r) {
      const tHit = tClosest - Math.sqrt(r * r - perp2);
      if (tHit < bestT) { bestT = tHit; best = bot; }
    }
  }
  if (!best) return null;
  return { bot: best, hitPx: fromPx + dirX * bestT, hitPy: fromPy + dirY * bestT };
}

// ── Player move ────────────────────────────────────────────────────────────────

function applyPlayerMove(gx, gy) {
  if (ME.health === 0) return;
  const dx = gx - ME.gx, dy = gy - ME.gy;
  if (dx === 0 && dy === 0) return;
  if (gx < 0 || gx >= COLS || gy < 0 || gy >= ROWS) return;
  if (isBarrier(gx, gy)) return;
  if (Math.sqrt(dx * dx + dy * dy) > MOVE_RADIUS) return;

  const v = playerVisuals[ME.id];
  if (v && (v.gridX !== gx || v.gridY !== gy)) {
    const fromGX = Math.round((v.px - CELL / 2) / CELL);
    const fromGY = Math.round((v.py - CELL / 2) / CELL);
    v.waypoints = computePath(fromGX, fromGY, gx, gy);
    v.gridX = gx; v.gridY = gy;
  }
  ME.gx = gx; ME.gy = gy;
  checkHealPickup(ME);
  checkDoorEntry();
}

// ── Player shoot ───────────────────────────────────────────────────────────────

function tryShootAt(gx, gy) {
  if (ME.reloading) return;
  if (ME.ammo === 0) { showReloadPopup(); return; }
  const now = Date.now();
  if (now - ME.lastShotAt < SHOT_COOLDOWN) return;
  ME.lastShotAt = now;

  const myV = playerVisuals[ME.id];
  if (!myV) return;

  const targetPx = gx * CELL + CELL / 2, targetPy = gy * CELL + CELL / 2;
  const edge = extendRayToEdge(myV.px, myV.py, targetPx, targetPy);
  const hit  = botOnRay(myV.px, myV.py, edge.x, edge.y);
  const toPx = hit ? hit.hitPx : edge.x;
  const toPy = hit ? hit.hitPy : edge.y;
  const dist = Math.sqrt((toPx - myV.px) ** 2 + (toPy - myV.py) ** 2);
  const duration = Math.max(100, dist / 2);

  const bullet = { fromPx: myV.px, fromPy: myV.py, toPx, toPy,
                   startTime: performance.now(), duration,
                   spawnImpactOnEnd: !!hit, blocked: false };
  bullets.push(bullet);
  ME.ammo--;
  updateAmmoBadge();

  if (hit) {
    setTimeout(() => {
      if (bullet.blocked) return; // bullet was stopped by a wall
      const bot = hit.bot;
      if (bot.health === 0) return;
      bot.health = Math.max(0, bot.health - 33);
      if (bot.health === 0) {
        onBotDied(bot);
      }
    }, duration);
  }
}

// ── Bot death callbacks ────────────────────────────────────────────────────────

// onBotDied is called immediately after a bot's health reaches zero.
function onBotDied(bot) {
  claimBotKillReward();

  if (bot.id === "bot1") {
    // Bot 1 died — wake bot 2 if still alive.
    const b2 = bots.find(b => b.id === "bot2");
    if (b2 && b2.health > 0) b2.forcedActive = true;
  } else if (bot.id === "bot2") {
    // Bot 2 died — wake bot 1 if still alive.
    const b1 = bots.find(b => b.id === "bot1");
    if (b1 && b1.health > 0) b1.forcedActive = true;
  } else if (bot.id === "bot3") {
    // Bot 3 died — give player 10 s to cross; otherwise force-wake bots 1 and 2.
    startBot3DeadCountdown();
  }

  checkRoundOver();
}

// startBot3DeadCountdown activates bots 1 and 2 after 10 s if the player has
// not yet crossed the center line.
function startBot3DeadCountdown() {
  if (bot3DeadTimer !== null) return;
  bot3DeadTimer = setTimeout(() => {
    bot3DeadTimer = null;
    if (bot3ReachedCenter) return; // player already crossed — nothing to do
    bot3ReachedCenter = true;
  }, 10000);
}

// ── Bot AI ─────────────────────────────────────────────────────────────────────

let lastBotShotAt = 0; // global cooldown so only one bot shoots at a time

// Bot 3 must reach the center column before bots 1 and 2 activate.
const CENTER_GX = Math.floor(COLS / 2); // col 12 — the visible dashed midline
let bot3ReachedCenter = false;
let bot3DeadTimer = null; // 10-s countdown started when bot 3 dies

function botTick() {
  const now = Date.now();
  const globalShotReady = now - lastBotShotAt >= BOT_GLOBAL_SHOT_CD;

  // Also activate bots 1 and 2 if the player ventures into the right half.
  if (!bot3ReachedCenter && ME.gx > CENTER_GX) {
    bot3ReachedCenter = true;
  }

  for (let bi = 0; bi < bots.length; bi++) {
    const bot = bots[bi];
    if (bot.health === 0) continue;

    // Bots 1 and 2 (indices 0 and 1) are frozen until globally or individually activated.
    if (bi < 2 && !bot3ReachedCenter && !bot.forcedActive) continue;

    const canShoot = globalShotReady && !bot.reloading && bot.ammo > 0 &&
                     (now - bot.lastShotAt >= BOT_SHOT_COOLDOWN);

    // Shoot the player when alive and in line of sight.
    if (ME.health > 0 && canShoot && hasLineOfSight(bot.gx, bot.gy, ME.gx, ME.gy)) {
      lastBotShotAt = now;
      botShoot(bot);
      continue;
    }

    // Move: chase the player, or wander when player is dead.
    if (now - bot.lastMoveAt >= BOT_MOVE_INTERVAL) {
      bot.lastMoveAt = now;
      if (ME.health > 0) {
        botChasePlayer(bot);
      } else {
        botWander(bot);
      }
      // Unlock bots 1 and 2 once Bot 3 crosses the center line.
      if (bi === 2 && !bot3ReachedCenter && bot.gx <= CENTER_GX) {
        bot3ReachedCenter = true;
      }
    }
  }
}

function botShoot(bot) {
  bot.lastShotAt = Date.now();
  bot.ammo--;
  if (bot.ammo === 0) {
    bot.reloading = true;
    setTimeout(() => { bot.reloading = false; bot.ammo = MAX_AMMO; }, RELOAD_TIME);
  }

  const bv = playerVisuals[bot.id];
  const pv = playerVisuals[ME.id];
  if (!bv || !pv) return;

  const dist = Math.sqrt((pv.px - bv.px) ** 2 + (pv.py - bv.py) ** 2);
  const duration = Math.max(100, dist / 2);
  const bullet = { fromPx: bv.px, fromPy: bv.py, toPx: pv.px, toPy: pv.py,
                   startTime: performance.now(), duration, spawnImpactOnEnd: false, blocked: false };
  bullets.push(bullet);

  setTimeout(() => {
    if (bullet.blocked) return; // bullet was stopped by a wall
    if (ME.health === 0) return;
    ME.health = Math.max(0, ME.health - 33);
    if (ME.health === 0) onPlayerDied();
  }, duration);
}

function botChasePlayer(bot) {
  const path = computePath(bot.gx, bot.gy, ME.gx, ME.gy);
  if (path.length === 0) return;
  const next = path[0];
  const v = playerVisuals[bot.id];
  if (v) {
    v.waypoints = [next];
    v.gridX = next.gx; v.gridY = next.gy;
  }
  bot.gx = next.gx; bot.gy = next.gy;
  checkHealPickup(bot);
}

function botWander(bot) {
  const dirs = [[-1,0],[1,0],[0,-1],[0,1]];
  const d = dirs[Math.floor(Math.random() * dirs.length)];
  const nx = bot.gx + d[0], ny = bot.gy + d[1];
  if (nx < 0 || nx >= COLS || ny < 0 || ny >= ROWS || isBarrier(nx, ny)) return;
  const v = playerVisuals[bot.id];
  if (v) {
    v.waypoints = [{ gx: nx, gy: ny }];
    v.gridX = nx; v.gridY = ny;
  }
  bot.gx = nx; bot.gy = ny;
}

// ── Round logic ────────────────────────────────────────────────────────────────

let roundActive = true;

function onPlayerDied() {
  if (!roundActive) return;
  roundActive = false;
  document.getElementById("death-overlay").classList.remove("hidden");
  setTimeout(() => {
    document.getElementById("death-overlay").classList.add("hidden");
    restartRound();
  }, 3000);
}

function checkRoundOver() {
  if (!roundActive) return;
  if (!bots.every(b => b.health === 0)) return;
  roundActive = false;
  const overlay = document.getElementById("round-overlay");
  document.getElementById("round-msg").textContent = "All bots eliminated!";
  overlay.classList.remove("hidden");
  setTimeout(() => {
    overlay.classList.add("hidden");
    restartRound();
  }, 3000);
}

function restartRound() {
  stage = 1;
  ME.health = 99; ME.gx = ME.spawnGX; ME.gy = ME.spawnGY;
  ME.ammo = MAX_AMMO; ME.reloading = false;
  clearTimeout(reloadTimer); clearTimeout(reloadCountdown);
  updateAmmoBadge();

  const meV = playerVisuals[ME.id];
  if (meV) { meV.gridX = ME.gx; meV.gridY = ME.gy; meV.waypoints = []; }

  // Drop the stage-2 extra bot and reset to the base 3-bot roster.
  delete playerVisuals[BOT4_DEF.id];
  initBots();
  for (const bot of bots) {
    ensureVisual(bot);
    const bv = playerVisuals[bot.id];
    if (bv) { bv.gridX = bot.gx; bv.gridY = bot.gy; bv.waypoints = []; }
  }

  lastBotShotAt = 0;
  bot3ReachedCenter = false;
  clearTimeout(bot3DeadTimer);
  bot3DeadTimer = null;
  roundActive = true;
}

// ── Keyboard ───────────────────────────────────────────────────────────────────

let lastMoveAt = 0;
const MOVE_COOLDOWN = 150;
let hoverCell = null;

document.addEventListener("keydown", (e) => {
  if (ME.health === 0) return;

  if (e.key === " ") {
    e.preventDefault();
    if (hoverCell) tryShootAt(hoverCell.gx, hoverCell.gy);
    return;
  }
  if (e.key === "r" || e.key === "R") { startReload(); return; }

  const now = Date.now();
  if (now - lastMoveAt < MOVE_COOLDOWN) return;
  let gx = ME.gx, gy = ME.gy;
  switch (e.key) {
    case "ArrowLeft":  case "a": gx--; break;
    case "ArrowRight": case "d": gx++; break;
    case "ArrowUp":    case "w": gy--; break;
    case "ArrowDown":  case "s": gy++; break;
    default: return;
  }
  e.preventDefault();
  lastMoveAt = now;
  applyPlayerMove(gx, gy);
});

// ── Mouse ──────────────────────────────────────────────────────────────────────

canvas.addEventListener("mousemove", (e) => {
  const rect = canvas.getBoundingClientRect();
  hoverCell = { gx: Math.floor((e.clientX - rect.left) / CELL),
                gy: Math.floor((e.clientY - rect.top)  / CELL) };
});
canvas.addEventListener("mouseleave", () => { hoverCell = null; });

canvas.addEventListener("click", (e) => {
  if (ME.health === 0) return;
  const rect = canvas.getBoundingClientRect();
  const gx = Math.floor((e.clientX - rect.left) / CELL);
  const gy = Math.floor((e.clientY - rect.top)  / CELL);
  tryShootAt(gx, gy);
});

// ── Visuals update ─────────────────────────────────────────────────────────────

function updateVisuals() {
  const allEntities = [ME, ...bots];
  for (const entity of allEntities) {
    const v = playerVisuals[entity.id];
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
}

// ── Draw ───────────────────────────────────────────────────────────────────────

function isReachable(ox, oy, gx, gy) {
  if (ox === gx && oy === gy) return false;
  if (isBarrier(gx, gy)) return false;
  const dx = gx - ox, dy = gy - oy;
  return Math.sqrt(dx * dx + dy * dy) <= MOVE_RADIUS;
}

function draw() {
  ctx.fillStyle = "#1a1a2e";
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  updateVisuals();
  drawGrid();
  if (ME.health > 0) drawReachableArea(ME);
  if (hoverCell && ME.health > 0 && isReachable(ME.gx, ME.gy, hoverCell.gx, hoverCell.gy))
    drawCellHighlight(hoverCell.gx, hoverCell.gy, "rgba(255,255,255,0.10)");
  drawBarriers();
  drawHealItems();
  drawDoor();
  const allEntities = [ME, ...bots];
  for (const entity of allEntities) {
    const v = playerVisuals[entity.id];
    if (v) drawPlayer(entity, v.px, v.py);
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
  const midX = (COLS / 2) * CELL;
  ctx.save();
  ctx.strokeStyle = "rgba(180,180,220,0.25)"; ctx.lineWidth = 2;
  ctx.setLineDash([8, 6]);
  ctx.beginPath(); ctx.moveTo(midX, 0); ctx.lineTo(midX, ROWS * CELL); ctx.stroke();
  ctx.restore();
}

function drawReachableArea(entity) {
  ctx.fillStyle = "rgba(74,144,217,0.06)";
  for (let gy = 0; gy < ROWS; gy++)
    for (let gx = 0; gx < COLS; gx++)
      if (isReachable(entity.gx, entity.gy, gx, gy))
        ctx.fillRect(gx * CELL, gy * CELL, CELL, CELL);
}

function drawCellHighlight(gx, gy, color) {
  ctx.fillStyle = color;
  ctx.fillRect(gx * CELL, gy * CELL, CELL, CELL);
}

function drawBarriers() {
  for (const key of BARRIERS) {
    const [gx, gy] = key.split(",").map(Number);
    const x = gx * CELL, y = gy * CELL;
    ctx.fillStyle = "#7a7a8a"; ctx.fillRect(x, y, CELL, CELL);
    ctx.fillStyle = "#5c5c6e"; ctx.fillRect(x + 5, y + 5, CELL - 10, CELL - 10);
    ctx.fillStyle = "#b0b0c8"; ctx.fillRect(x, y, CELL, 5); ctx.fillRect(x, y, 5, CELL);
    ctx.fillStyle = "#2a2a3a"; ctx.fillRect(x, y + CELL - 5, CELL, 5); ctx.fillRect(x + CELL - 5, y, 5, CELL);
  }
}

function drawDoor() {
  if (stage !== 1) return;
  const x = DOOR_GX * CELL, y = DOOR_GY * CELL;
  // Background glow
  ctx.fillStyle = "#1a4a1a";
  ctx.fillRect(x, y, CELL, CELL);
  ctx.fillStyle = "#2a7a2a";
  ctx.fillRect(x + 3, y + 3, CELL - 6, CELL - 6);
  // Arrow
  ctx.fillStyle = "#52C07A";
  ctx.font = "bold 20px system-ui";
  ctx.textAlign = "center";
  ctx.textBaseline = "middle";
  ctx.fillText("▶", x + CELL / 2, y + CELL / 2);
}

function drawHealItems() {
  for (const item of healItems) {
    const x = item.gx * CELL, y = item.gy * CELL;
    const pad = 6, cx = x + CELL / 2, cy = y + CELL / 2;
    const arm = 7, thick = 4;
    ctx.fillStyle = "#ffffff";
    ctx.fillRect(x + pad, y + pad, CELL - pad * 2, CELL - pad * 2);
    ctx.fillStyle = "#e05252";
    ctx.fillRect(cx - arm, cy - thick / 2, arm * 2, thick);
    ctx.fillRect(cx - thick / 2, cy - arm, thick, arm * 2);
  }
}

function drawPlayer(entity, px, py) {
  const r     = CELL / 2 - 2;
  const isMe  = entity.id === "player";
  const dead  = entity.health === 0;
  const incap = entity.health === 33;
  ctx.save();
  if (dead) ctx.globalAlpha = 0.4;
  else if (incap) ctx.globalAlpha = 0.55;
  ctx.beginPath();
  ctx.arc(px, py, r, 0, Math.PI * 2);
  ctx.fillStyle = dead ? "#666" : entity.color;
  ctx.fill();
  const teamColor = entity.team === "red" ? "#c0392b" : "#2471a3";
  ctx.strokeStyle = isMe ? "#ffffff" : teamColor;
  ctx.lineWidth   = isMe ? 2.5 : 2;
  ctx.stroke();
  ctx.fillStyle = "#ffffff"; ctx.font = "bold 15px system-ui";
  ctx.textAlign = "center"; ctx.textBaseline = "middle";
  ctx.fillText(dead ? "✕" : "Ӿ", px, py);
  ctx.restore();
  drawHealthBar(px, py - r - 8, entity.health);
  ctx.save();
  ctx.font = "bold 9px system-ui"; ctx.textAlign = "center"; ctx.textBaseline = "top";
  ctx.fillStyle = "rgba(0,0,0,0.55)";
  ctx.fillRect(px - 20, py + r + 3, 40, 11);
  ctx.fillStyle = entity.team === "red" ? "#e88" : "#8af";
  ctx.fillText(entity.nickname.slice(0, 12), px, py + r + 4);
  ctx.restore();
}

function drawHealthBar(cx, cy, health) {
  const w = CELL - 4, h = 4, x = cx - w / 2;
  const pct = Math.max(0, health / 99);
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
    ctx.save(); ctx.globalAlpha = 1 - t;
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
    const r = 6 + t * 8;
    ctx.save(); ctx.globalAlpha = 1 - t;
    ctx.beginPath(); ctx.arc(f.px, f.py, r, 0, Math.PI * 2);
    ctx.strokeStyle = "#FFD700"; ctx.lineWidth = 2; ctx.stroke();
    ctx.beginPath(); ctx.arc(f.px, f.py, 3 * (1 - t), 0, Math.PI * 2);
    ctx.fillStyle = "#ffffff"; ctx.fill();
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
    if (isBarrier(Math.floor(bpx / CELL), Math.floor(bpy / CELL))) {
      b.blocked = true; // abort any pending damage setTimeout for this bullet
      wallImpacts.push({ px: bpx, py: bpy, startTime: performance.now(), duration: 250 });
      bullets.splice(i, 1); continue;
    }
    ctx.beginPath(); ctx.arc(bpx, bpy, 4, 0, Math.PI * 2);
    ctx.fillStyle = "#FFD700"; ctx.fill();
    if (t >= 1) {
      if (b.spawnImpactOnEnd)
        wallImpacts.push({ px: b.toPx, py: b.toPy, startTime: performance.now(), duration: 300 });
      bullets.splice(i, 1);
    } else i++;
  }
}

// ── Start ──────────────────────────────────────────────────────────────────────

initBots();
ensureVisual(ME);
for (const bot of bots) ensureVisual(bot);
spawnHealItems(HEAL_ITEM_COUNT);
updateAmmoBadge();
setInterval(botTick, 100); // check bots every 100ms; each bot respects its own cooldowns
draw();
